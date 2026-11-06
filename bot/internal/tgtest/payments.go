package tgtest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
)

var errCommit = errors.New("tgtest: commit failed")

// Payments — in-memory фейк store для тестов хендлеров; правила UserStats как в SQL.
type Payments struct {
	mu             sync.Mutex
	payments       []model.Payment // индекс = ID-1
	outbox         []model.OutboxMessage
	failResolve    bool
	failSubmitOnce bool
	failAdvance    bool
	failDaysOnce   bool
	blocked        map[int64]bool // ручная блокировка админом
	loginTokens    []LoginToken
	promos         []model.Promo // индекс = ID-1
	activations    []model.PromoActivation
	extra          map[int64]int       // users.extra_devices
	trial          map[int64]time.Time // users.trial_used_at
	grants         []Grant
	failTrialOnce  bool
	gifts          []model.Gift // индекс = ID-1

	// Отдельный мьютекс: панель зовут из apply под p.mu. Под ним же рефералка: users (кто уже есть), referred_by, награды.
	userMu     sync.Mutex
	touched    []model.UserSeen
	panel      []PanelWrite
	known      map[int64]bool
	referredBy map[int64]int64
	rewards    map[int64]*Reward
}

// Reward — строка referral_rewards.
type Reward struct {
	Referrer, Days   int64
	FriendRewarded   bool
	ReferrerRewarded bool
}

// Grant — строка grants пробного периода (источник trial).
type Grant struct {
	TelegramID int64
	Days       int
	Source     string
}

// PanelWrite — запись SetUserPanel.
type PanelWrite struct {
	TelegramID int64
	ExpireAt   time.Time
	Status     string
}

func (p *Payments) SetUserPanel(_ context.Context, id int64, expireAt time.Time, status string, _ time.Time) error {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	p.panel = append(p.panel, PanelWrite{TelegramID: id, ExpireAt: expireAt, Status: status})
	return nil
}

// PanelWrites — записи SetUserPanel по порядку.
func (p *Payments) PanelWrites() []PanelWrite {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	return slices.Clone(p.panel)
}

func (p *Payments) TouchUser(_ context.Context, u model.UserSeen) error {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	p.touched = append(p.touched, u)
	p.markKnown(u.TelegramID)
	return nil
}

func (p *Payments) markKnown(id int64) {
	if p.known == nil {
		p.known = map[int64]bool{}
	}
	p.known[id] = true
}

// SeedUser — юзер уже есть в users (как после первого касания).
func (p *Payments) SeedUser(id int64) {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	p.markKnown(id)
}

// RecordReferral — как store: запись только для нового юзера, пригласивший должен быть и не быть самим юзером.
func (p *Payments) RecordReferral(_ context.Context, u model.UserSeen, referrerID int64) (bool, error) {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	if p.known[u.TelegramID] || !p.known[referrerID] || referrerID == u.TelegramID {
		return false, nil
	}
	p.markKnown(u.TelegramID)
	if p.referredBy == nil {
		p.referredBy = map[int64]int64{}
	}
	p.referredBy[u.TelegramID] = referrerID
	return true, nil
}

// ReferredBy — кто пригласил юзера.
func (p *Payments) ReferredBy(id int64) (int64, bool) {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	r, ok := p.referredBy[id]
	return r, ok
}

// Rewards — награды рефералки по приглашённому.
func (p *Payments) Rewards() map[int64]Reward {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	out := map[int64]Reward{}
	for k, v := range p.rewards {
		out[k] = *v
	}
	return out
}

// ResolvePaymentRef — ResolvePayment с рефералкой по правилам store.ResolvePaymentRef (в упрощённом виде, без параллельности).
func (p *Payments) ResolvePaymentRef(
	ctx context.Context, id int64, status model.PaymentStatus, at time.Time, days int, apply func(model.Payment) ([]model.OutboxMessage, error),
) (model.Payment, error) {
	if days <= 0 || status != model.PaymentConfirmed {
		return p.ResolvePayment(ctx, id, status, at, apply)
	}
	var referrer int64
	reward, friendDays := false, false
	res, err := p.ResolvePayment(ctx, id, status, at, func(x model.Payment) ([]model.OutboxMessage, error) {
		p.userMu.Lock()
		r, ok := p.referredBy[x.TelegramID]
		_, done := p.rewards[x.TelegramID]
		p.userMu.Unlock()
		if ok && !done && x.Kind == model.KindTariff && x.Price > 0 &&
			!slices.ContainsFunc(p.payments, func(o model.Payment) bool {
				return o.TelegramID == x.TelegramID && o.Status == model.PaymentConfirmed && o.Kind == model.KindTariff && o.Price > 0 && o.ID != x.ID
			}) {
			referrer, reward = r, true
			if friendDays = !p.blocked[x.TelegramID]; friendDays {
				x.ReferralFriendDays = days
			}
		}
		return apply(x)
	})
	if err != nil || !reward {
		return res, err
	}
	p.userMu.Lock()
	defer p.userMu.Unlock()
	if p.rewards == nil {
		p.rewards = map[int64]*Reward{}
	}
	p.rewards[res.TelegramID] = &Reward{Referrer: referrer, Days: int64(days), FriendRewarded: friendDays}
	if friendDays {
		p.grants = append(p.grants, Grant{TelegramID: res.TelegramID, Days: days, Source: "referral"})
	}
	res.ReferralPending = true
	return res, nil
}

func (p *Payments) isBlocked(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocked[id]
}

// GrantReferral — как store.GrantReferral; apply зовётся без мьютексов (из него идёт запись в users через панель).
func (p *Payments) GrantReferral(
	_ context.Context, referredID int64, now time.Time, apply func(int64, string, int) ([]model.OutboxMessage, error),
) error {
	p.userMu.Lock()
	r, ok := p.rewards[referredID]
	p.userMu.Unlock()
	switch {
	case !ok:
		return store.ErrNotFound
	case r.ReferrerRewarded:
		return store.ErrAlreadyRewarded
	case p.isBlocked(r.Referrer):
		return store.ErrBlocked
	}
	msgs, err := apply(r.Referrer, "referrer", int(r.Days))
	if err != nil {
		return err
	}
	p.mu.Lock()
	if i := p.pendingIdx(r.Referrer); i >= 0 {
		x := &p.payments[i]
		x.PrevExpireAt = later(x.PrevExpireAt, now).Add(time.Duration(r.Days) * 24 * time.Hour)
	}
	p.outbox = append(p.outbox, msgs...)
	p.mu.Unlock()
	p.userMu.Lock()
	defer p.userMu.Unlock()
	r.ReferrerRewarded = true
	p.grants = append(p.grants, Grant{TelegramID: r.Referrer, Days: int(r.Days), Source: "referral"})
	return nil
}

// Touched — записи TouchUser по порядку.
func (p *Payments) Touched() []model.UserSeen {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	return slices.Clone(p.touched)
}

// LoginToken — токен входа, созданный через CreateLoginToken.
type LoginToken struct {
	Hash      []byte
	ExpiresAt time.Time
}

func NewPayments() *Payments {
	return &Payments{}
}

// FailResolveCommitOnce: следующий ResolvePayment выполнит apply, затем вернёт ошибку без смены статуса.
func (p *Payments) FailResolveCommitOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failResolve = true
}

// FailDaysCommitOnce: следующий ActivateDaysPromo выполнит apply, затем вернёт ошибку без активации.
func (p *Payments) FailDaysCommitOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failDaysOnce = true
}

// FailSubmitNotPendingOnce: следующий SubmitPayment вернёт store.ErrNotPending.
func (p *Payments) FailSubmitNotPendingOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failSubmitOnce = true
}

// SetBlocked ставит или снимает ручную блокировку юзера (users.blocked_at).
func (p *Payments) SetBlocked(id int64, blocked bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.blocked == nil {
		p.blocked = map[int64]bool{}
	}
	p.blocked[id] = blocked
}

func (p *Payments) UserBlocked(_ context.Context, id int64) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.blocked[id], nil
}

// FailGrantAdvance: GrantAdvance возвращает ошибку, пока не снят (false).
func (p *Payments) FailGrantAdvance(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failAdvance = fail
}

func (p *Payments) All() []model.Payment {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.payments)
}

func (p *Payments) Outbox() []model.OutboxMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.outbox)
}

func (p *Payments) pendingIdx(telegramID int64) int {
	return slices.IndexFunc(p.payments, func(x model.Payment) bool {
		return x.TelegramID == telegramID && x.Status == model.PaymentPending
	})
}

func (p *Payments) byID(id int64) (*model.Payment, error) {
	if id < 1 || id > int64(len(p.payments)) {
		return nil, store.ErrNotFound
	}
	return &p.payments[id-1], nil
}

// CreatePayment — rollback вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) CreatePayment(
	ctx context.Context, np model.Payment, rollback func(old model.Payment) error,
	readPrev func(ctx context.Context) (time.Time, error),
) (model.Payment, *model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if np.PromoID != nil && p.blocked[np.TelegramID] {
		return model.Payment{}, nil, promo.ErrBlocked
	}
	if np.Kind == "" {
		np.Kind = model.KindTariff
	}
	var cancelled *model.Payment
	if i := p.pendingIdx(np.TelegramID); i >= 0 {
		old := &p.payments[i]
		if np.Kind != model.KindTariff {
			return model.Payment{}, nil, store.ErrPendingExists
		}
		if err := rollback(*old); err != nil {
			return model.Payment{}, nil, err
		}
		at := time.Now()
		old.Status, old.ResolvedAt = model.PaymentCancelled, &at
		p.setGiftStatus(old.ID, model.GiftCancelled)
		c := *old
		cancelled = &c
		if old.Kind == model.KindTariff {
			np.PrevExpireAt = old.PrevExpireAt
		} else if readPrev != nil {
			prev, err := readPrev(ctx)
			if err != nil {
				return model.Payment{}, nil, err
			}
			np.PrevExpireAt = prev
		}
	} else if readPrev != nil {
		prev, err := readPrev(ctx)
		if err != nil {
			return model.Payment{}, nil, err
		}
		np.PrevExpireAt = prev
	}
	np.ID = int64(len(p.payments) + 1)
	np.Status = model.PaymentPending
	np.CreatedAt = time.Now()
	np.AdvanceGranted, np.AdminMessageID, np.SubmittedAt, np.ResolvedAt = false, 0, nil, nil
	p.payments = append(p.payments, np)
	if np.Kind == model.KindGift {
		id, buyer := np.ID, np.TelegramID
		p.gifts = append(p.gifts, model.Gift{
			ID: int64(len(p.gifts) + 1), Code: fmt.Sprintf("fakegiftcode%010d", len(p.gifts)+1), Months: np.Month,
			Status: model.GiftPending, BuyerID: &buyer, PaymentID: &id,
		})
	}
	return np, cancelled, nil
}

// setGiftStatus — статус подарка платежа paymentID (pending-подарок отменённого или решённого платежа).
func (p *Payments) setGiftStatus(paymentID int64, st model.GiftStatus) {
	for i := range p.gifts {
		if p.gifts[i].PaymentID != nil && *p.gifts[i].PaymentID == paymentID {
			p.gifts[i].Status = st
		}
	}
}

// SeedGift заводит готовый подарок (как выданный админом, если buyer == 0) и возвращает его код.
func (p *Payments) SeedGift(code string, months int, buyer int64, st model.GiftStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	g := model.Gift{ID: int64(len(p.gifts) + 1), Code: code, Months: months, Status: st, ByAdmin: buyer == 0}
	if buyer != 0 {
		g.BuyerID = &buyer
	}
	p.gifts = append(p.gifts, g)
}

// Gifts — подарки по порядку создания.
func (p *Payments) Gifts() []model.Gift {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.gifts)
}

// RedeemGift — как store.RedeemGift: любой отказ — ErrGiftUnavailable; apply под мьютексом фейка (из него Payments звать нельзя).
func (p *Payments) RedeemGift(
	_ context.Context, code string, id int64, now time.Time, apply func(model.Gift) ([]model.OutboxMessage, error),
) (model.Gift, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := slices.IndexFunc(p.gifts, func(g model.Gift) bool { return g.Code == code })
	if i < 0 || p.gifts[i].Status != model.GiftReady || p.blocked[id] {
		return model.Gift{}, model.ErrGiftUnavailable
	}
	g := &p.gifts[i]
	days := g.Months * 30
	if j := p.pendingIdx(id); j >= 0 && p.payments[j].Kind == model.KindTariff {
		x := &p.payments[j]
		x.PrevExpireAt = later(x.PrevExpireAt, now).Add(time.Duration(days) * 24 * time.Hour)
	}
	msgs, err := apply(*g)
	if err != nil {
		return model.Gift{}, err
	}
	g.Status, g.RedeemedBy, g.RedeemedAt = model.GiftRedeemed, &id, &now
	p.grants = append(p.grants, Grant{TelegramID: id, Days: days, Source: "gift"})
	p.outbox = append(p.outbox, msgs...)
	return *g, nil
}

func (p *Payments) PendingPayment(_ context.Context, telegramID int64) (model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := p.pendingIdx(telegramID)
	if i < 0 {
		return model.Payment{}, store.ErrNotFound
	}
	return p.payments[i], nil
}

func (p *Payments) PaymentByID(_ context.Context, id int64) (model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	x, err := p.byID(id)
	if err != nil {
		return model.Payment{}, err
	}
	return *x, nil
}

func (p *Payments) SubmitPayment(_ context.Context, id int64, adminMessageID int, photoFileID string, at time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failSubmitOnce {
		p.failSubmitOnce = false
		return store.ErrNotPending
	}
	x, err := p.byID(id)
	if err != nil || x.Status != model.PaymentPending || x.SubmittedAt != nil {
		return store.ErrNotPending
	}
	x.AdminMessageID, x.SubmittedAt, x.PhotoFileID = adminMessageID, &at, photoFileID
	return nil
}

// GrantAdvance — apply вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) GrantAdvance(_ context.Context, id int64, apply func(model.Payment)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failAdvance {
		return errCommit
	}
	x, err := p.byID(id)
	if err != nil {
		return err
	}
	if x.Status != model.PaymentPending {
		return store.ErrNotPending
	}
	if p.blocked[x.TelegramID] {
		return store.ErrBlocked
	}
	x.AdvanceGranted = true
	apply(*x)
	return nil
}

func (p *Payments) UserStats(_ context.Context, telegramID, excludeID int64) (model.UserStats, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var (
		st   model.UserStats
		last int64
	)
	for _, x := range p.payments {
		if x.TelegramID != telegramID || x.ID == excludeID {
			continue
		}
		st.HasHistory = true
		if x.Status != model.PaymentConfirmed {
			continue
		}
		if x.Kind == model.KindTariff {
			last = max(last, x.ID)
		}
		st.Confirmed++
		st.ConfirmedSum += x.Price
		at := x.CreatedAt
		if x.ResolvedAt != nil {
			at = *x.ResolvedAt
		}
		if st.LastConfirmedAt == nil || at.After(*st.LastConfirmedAt) {
			st.LastConfirmedAt = &at
		}
		if st.FirstConfirmedAt == nil || at.Before(*st.FirstConfirmedAt) {
			st.FirstConfirmedAt = &at
		}
	}
	st.Blocked = p.blocked[telegramID]
	if _, ok := p.trial[telegramID]; ok && last == 0 {
		st.UnpaidAdvances++ // пробный — один неоплаченный аванс до первой подтверждённой подписки
	}
	for _, x := range p.payments {
		if x.TelegramID == telegramID && x.ID != excludeID && x.ID > last && x.AdvanceGranted &&
			(x.Status == model.PaymentDeclined || x.Status == model.PaymentCancelled) {
			st.UnpaidAdvances++
		}
	}
	return st, nil
}

// ResolvePayment — apply вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) ResolvePayment(
	_ context.Context, id int64, status model.PaymentStatus, at time.Time, apply func(model.Payment) ([]model.OutboxMessage, error),
) (model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if status != model.PaymentConfirmed && status != model.PaymentDeclined {
		return model.Payment{}, store.ErrInvalidStatus
	}
	x, err := p.byID(id)
	if err != nil {
		return model.Payment{}, err
	}
	if x.Status != model.PaymentPending {
		return *x, store.ErrNotPending
	}
	// как в store: активация до apply; оплаченный — без проверки лимита, повтор — ON CONFLICT DO NOTHING;
	// цена 0 — заново promo.Check (лимит, повтор → ErrAlreadyUsed)
	activate := false
	if status == model.PaymentConfirmed && x.PromoID != nil {
		used, already := p.usage(*x.PromoID, x.TelegramID)
		if x.Price == 0 {
			if *x.PromoID < 1 || *x.PromoID > int64(len(p.promos)) {
				return model.Payment{}, errors.Join(promo.ErrNotFound, store.ErrNotFound)
			}
			if p.blocked[x.TelegramID] {
				return model.Payment{}, promo.ErrBlocked
			}
			if err := promo.Check(p.promos[*x.PromoID-1], used, x.TelegramID, p.hasConfirmed(x.TelegramID), already,
				promo.PlacePayment, at); err != nil {
				return model.Payment{}, err
			}
		}
		activate = !already
	}
	cur := p.extra[x.TelegramID]
	x.UserExtra = cur
	if x.Kind == model.KindGift {
		for _, g := range p.gifts {
			if g.PaymentID != nil && *g.PaymentID == x.ID {
				x.GiftCode = g.Code
			}
		}
	}
	_, tr := p.trial[x.TelegramID]
	x.TrialConvert = tr && !p.hasConfirmed(x.TelegramID)
	msgs, err := apply(*x)
	if err != nil {
		return model.Payment{}, err
	}
	if p.failResolve {
		p.failResolve = false
		return model.Payment{}, errCommit
	}
	x.Status, x.ResolvedAt = status, &at
	if x.Kind == model.KindGift {
		next := model.GiftCancelled
		if status == model.PaymentConfirmed {
			next = model.GiftReady
		}
		p.setGiftStatus(x.ID, next)
	}
	if status == model.PaymentConfirmed && x.ExtraAfter(cur) != cur {
		p.setExtra(x.TelegramID, x.ExtraAfter(cur))
	}
	p.outbox = append(p.outbox, msgs...)
	if activate {
		id := x.ID
		p.activations = append(p.activations, model.PromoActivation{PromoID: *x.PromoID, TelegramID: x.TelegramID, PaymentID: &id})
	}
	return *x, nil
}

func (p *Payments) CreateLoginToken(_ context.Context, hash []byte, expiresAt time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loginTokens = append(p.loginTokens, LoginToken{Hash: hash, ExpiresAt: expiresAt})
	return nil
}

func (p *Payments) LoginTokens() []LoginToken {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.loginTokens)
}

// AddPromo заводит промокод (код нормализуется, как в store) и возвращает его с ID.
func (p *Payments) AddPromo(pr model.Promo) model.Promo {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr.ID = int64(len(p.promos) + 1)
	pr.Code = promo.Normalize(pr.Code)
	p.promos = append(p.promos, pr)
	return pr
}

func (p *Payments) Activations() []model.PromoActivation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.activations)
}

func (p *Payments) PromoByCode(_ context.Context, code string) (model.Promo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := slices.IndexFunc(p.promos, func(x model.Promo) bool { return x.Code == promo.Normalize(code) })
	if i < 0 {
		return model.Promo{}, store.ErrNotFound
	}
	return p.promos[i], nil
}

func (p *Payments) PromoByID(_ context.Context, id int64) (model.Promo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id < 1 || id > int64(len(p.promos)) {
		return model.Promo{}, store.ErrNotFound
	}
	return p.promos[id-1], nil
}

func (p *Payments) usage(promoID, telegramID int64) (used int, already bool) {
	for _, a := range p.activations {
		if a.PromoID == promoID {
			used++
			already = already || a.TelegramID == telegramID
		}
	}
	return used, already
}

func (p *Payments) PromoUsage(_ context.Context, promoID, telegramID int64) (int, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	used, already := p.usage(promoID, telegramID)
	return used, already, nil
}

func (p *Payments) hasConfirmed(telegramID int64) bool {
	return slices.ContainsFunc(p.payments, func(x model.Payment) bool {
		return x.TelegramID == telegramID && x.Status == model.PaymentConfirmed && x.Kind == model.KindTariff
	})
}

func (p *Payments) setExtra(id int64, n int) {
	if p.extra == nil {
		p.extra = map[int64]int{}
	}
	p.extra[id] = n
}

// SetExtraDevices задаёт купленные юзером устройства (users.extra_devices).
func (p *Payments) SetExtraDevices(id int64, n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setExtra(id, n)
}

func (p *Payments) ExtraDevices(_ context.Context, id int64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.extra[id], nil
}

func (p *Payments) HasConfirmed(_ context.Context, telegramID int64) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hasConfirmed(telegramID), nil
}

// ActivateDaysPromo — apply вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) ActivateDaysPromo(
	_ context.Context, promoID, telegramID int64, now time.Time, apply func(model.Promo) error,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if promoID < 1 || promoID > int64(len(p.promos)) {
		return errors.Join(promo.ErrNotFound, store.ErrNotFound)
	}
	pr := p.promos[promoID-1]
	if p.blocked[telegramID] {
		return promo.ErrBlocked
	}
	used, already := p.usage(promoID, telegramID)
	if err := promo.Check(pr, used, telegramID, p.hasConfirmed(telegramID), already, promo.PlaceMenu, now); err != nil {
		return err
	}
	if err := apply(pr); err != nil {
		return err
	}
	if p.failDaysOnce {
		p.failDaysOnce = false
		return errCommit
	}
	// как в store: база pending-платежа сдвигается на те же дни
	if i := p.pendingIdx(telegramID); i >= 0 {
		x := &p.payments[i]
		x.PrevExpireAt = later(x.PrevExpireAt, now).Add(time.Duration(pr.Value) * 24 * time.Hour)
	}
	p.activations = append(p.activations, model.PromoActivation{PromoID: promoID, TelegramID: telegramID})
	return nil
}

func (p *Payments) CancelPayment(_ context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	x, err := p.byID(id)
	if err != nil || x.Status != model.PaymentPending {
		return store.ErrNotPending
	}
	at := time.Now()
	x.Status, x.ResolvedAt = model.PaymentCancelled, &at
	p.setGiftStatus(x.ID, model.GiftCancelled)
	return nil
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// FailTrialCommitOnce: следующий StartTrial выполнит apply, затем вернёт ошибку без записи пробного.
func (p *Payments) FailTrialCommitOnce() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failTrialOnce = true
}

// TrialUsedAt — когда выдан пробный (users.trial_used_at); false — не выдавался.
func (p *Payments) TrialUsedAt(id int64) (time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	at, ok := p.trial[id]
	return at, ok
}

// SetTrialUsed помечает юзера взявшим пробный.
func (p *Payments) SetTrialUsed(id int64, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.trial == nil {
		p.trial = map[int64]time.Time{}
	}
	p.trial[id] = at
}

// Grants — строки grants пробного периода.
func (p *Payments) Grants() []Grant {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.grants)
}

// trialAllowed — как store.trialAllowed; вызывается под p.mu.
func (p *Payments) trialAllowed(id int64, advanceLimit int) bool {
	if _, used := p.trial[id]; used || p.blocked[id] {
		return false
	}
	for _, x := range p.payments {
		if x.TelegramID == id && x.Status == model.PaymentConfirmed && x.Kind != model.KindGift {
			return false
		}
	}
	unpaid := 0
	var last int64
	for _, x := range p.payments {
		if x.TelegramID == id && x.Status == model.PaymentConfirmed && x.Kind == model.KindTariff {
			last = max(last, x.ID)
		}
	}
	for _, x := range p.payments {
		if x.TelegramID == id && x.ID > last && x.AdvanceGranted && (x.Status == model.PaymentDeclined || x.Status == model.PaymentCancelled) {
			unpaid++
		}
	}
	return advanceLimit <= 0 || unpaid < advanceLimit
}

func (p *Payments) TrialAllowed(_ context.Context, id int64, advanceLimit int) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.trialAllowed(id, advanceLimit), nil
}

// StartTrial — apply вызывается под мьютексом фейка: из него нельзя звать Payments.
func (p *Payments) StartTrial(
	_ context.Context, id int64, _ string, days, advanceLimit int, now time.Time, apply func() ([]model.OutboxMessage, error),
) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.trialAllowed(id, advanceLimit) {
		return model.ErrTrialUnavailable
	}
	if i := p.pendingIdx(id); i >= 0 {
		x := &p.payments[i]
		x.PrevExpireAt = later(x.PrevExpireAt, now).Add(time.Duration(days) * 24 * time.Hour)
	}
	msgs, err := apply()
	if err != nil {
		return err
	}
	if p.failTrialOnce {
		p.failTrialOnce = false
		return errCommit
	}
	if p.trial == nil {
		p.trial = map[int64]time.Time{}
	}
	p.trial[id] = now
	p.grants = append(p.grants, Grant{TelegramID: id, Days: days, Source: "trial"})
	p.outbox = append(p.outbox, msgs...)
	return nil
}
