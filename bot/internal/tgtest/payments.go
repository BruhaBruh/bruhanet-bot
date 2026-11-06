package tgtest

import (
	"context"
	"errors"
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
	loginTokens    []LoginToken
	promos         []model.Promo // индекс = ID-1
	activations    []model.PromoActivation

	// Отдельный мьютекс: панель зовут из apply под p.mu.
	userMu  sync.Mutex
	touched []model.UserSeen
}

func (p *Payments) TouchUser(_ context.Context, u model.UserSeen) error {
	p.userMu.Lock()
	defer p.userMu.Unlock()
	p.touched = append(p.touched, u)
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

// FailMarkAdvance: MarkAdvance возвращает ошибку, пока не снят (false).
func (p *Payments) FailMarkAdvance(fail bool) {
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
	_ context.Context, np model.Payment, rollback func(old model.Payment) error,
) (model.Payment, *model.Payment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var cancelled *model.Payment
	if i := p.pendingIdx(np.TelegramID); i >= 0 {
		old := &p.payments[i]
		if err := rollback(*old); err != nil {
			return model.Payment{}, nil, err
		}
		at := time.Now()
		old.Status, old.ResolvedAt = model.PaymentCancelled, &at
		c := *old
		cancelled = &c
		np.PrevExpireAt = old.PrevExpireAt
	}
	np.ID = int64(len(p.payments) + 1)
	np.Status = model.PaymentPending
	np.CreatedAt = time.Now()
	np.AdvanceGranted, np.AdminMessageID, np.SubmittedAt, np.ResolvedAt = false, 0, nil, nil
	p.payments = append(p.payments, np)
	return np, cancelled, nil
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

func (p *Payments) SubmitPayment(_ context.Context, id int64, adminMessageID int, at time.Time) error {
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
	x.AdminMessageID, x.SubmittedAt = adminMessageID, &at
	return nil
}

func (p *Payments) MarkAdvance(_ context.Context, id int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failAdvance {
		return errCommit
	}
	x, err := p.byID(id)
	if err != nil {
		return err
	}
	x.AdvanceGranted = true
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
		last = max(last, x.ID)
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
	_ context.Context, id int64, status model.PaymentStatus, at time.Time, apply func(model.Payment) (model.OutboxMessage, error),
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
			if err := promo.Check(p.promos[*x.PromoID-1], used, x.TelegramID, p.hasConfirmed(x.TelegramID), already,
				promo.PlacePayment, at); err != nil {
				return model.Payment{}, err
			}
		}
		activate = !already
	}
	msg, err := apply(*x)
	if err != nil {
		return model.Payment{}, err
	}
	if p.failResolve {
		p.failResolve = false
		return model.Payment{}, errCommit
	}
	x.Status, x.ResolvedAt = status, &at
	p.outbox = append(p.outbox, msg)
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
		return x.TelegramID == telegramID && x.Status == model.PaymentConfirmed
	})
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
	return nil
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
