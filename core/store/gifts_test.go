package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

// limitPanel считает записи лимита устройств поверх refPanel.
type limitPanel struct {
	*refPanel
	mu     sync.Mutex
	limits int
}

func (p *limitPanel) SetDeviceLimit(ctx context.Context, id int64, l int) (*api.UserItemInfo, error) {
	p.mu.Lock()
	p.limits++
	p.mu.Unlock()
	return p.refPanel.SetDeviceLimit(ctx, id, l)
}

type giftFixture struct {
	s     *Store
	panel *limitPanel
	now   time.Time
}

func newGiftFixture(t *testing.T) *giftFixture {
	t.Helper()
	return &giftFixture{s: testStore(t), panel: &limitPanel{refPanel: newRefPanel()}, now: time.Now().Truncate(time.Microsecond)}
}

func (f *giftFixture) touch(t *testing.T, id int64) {
	t.Helper()
	if err := f.s.TouchUser(t.Context(), model.UserSeen{TelegramID: id, Username: "u", At: f.now}); err != nil {
		t.Fatal(err)
	}
}

func (f *giftFixture) buy(t *testing.T, buyer int64, months int) model.Payment {
	t.Helper()
	p, _, err := f.s.CreatePayment(t.Context(), model.Payment{
		TelegramID: buyer, Username: "buyer", Month: months, Price: 500, PrevExpireAt: f.now, Kind: model.KindGift,
	}, noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *giftFixture) resolve(ctx context.Context, id int64, approve bool) (model.Payment, error) {
	return payflow.Resolve(ctx, f.s, f.panel, payflow.Decision{ID: id, Approve: approve, At: f.now, DevicesBase: 5, ReferralDays: 7})
}

func (f *giftFixture) code(t *testing.T, paymentID int64) (code string) {
	t.Helper()
	if err := f.s.pool.QueryRow(t.Context(), `SELECT code FROM gifts WHERE payment_id = $1`, paymentID).Scan(&code); err != nil {
		t.Fatal(err)
	}
	return code
}

// ready — купленный и подтверждённый подарок: его код.
func (f *giftFixture) ready(t *testing.T, buyer int64, months int) string {
	t.Helper()
	p := f.buy(t, buyer, months)
	if _, err := f.resolve(t.Context(), p.ID, true); err != nil {
		t.Fatal(err)
	}
	return f.code(t, p.ID)
}

func (f *giftFixture) redeem(ctx context.Context, code string, id int64) (model.Gift, time.Time, error) {
	return payflow.RedeemGift(ctx, f.s, f.panel, payflow.GiftRedeem{Code: code, TelegramID: id, Username: "r", At: f.now, Squad: uuid.Nil})
}

func (f *giftFixture) count(t *testing.T, q string, args ...any) (n int) {
	t.Helper()
	if err := f.s.pool.QueryRow(t.Context(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *giftFixture) status(t *testing.T, code string) (st model.GiftStatus) {
	t.Helper()
	if err := f.s.pool.QueryRow(t.Context(), `SELECT status FROM gifts WHERE code = $1`, code).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

func (f *giftFixture) outbox(t *testing.T) []model.OutboxItem {
	t.Helper()
	items, err := f.s.OutboxBatch(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestGiftCodeEntropy(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		c, err := NewGiftCode()
		if err != nil || len(c) != 22 || seen[c] {
			t.Fatalf("code %q err=%v dup=%v", c, err, seen[c])
		}
		seen[c] = true
	}
}

// Подтверждение подарка: ссылка открыта, у покупателя ни срок, ни устройства, ни панель не тронуты.
func TestGiftPurchaseConfirmKeepsBuyerUntouched(t *testing.T) {
	f := newGiftFixture(t)
	f.touch(t, 1)
	if _, err := f.s.pool.Exec(t.Context(), `UPDATE users SET extra_devices = 2, trial_used_at = now() WHERE telegram_id = 1`); err != nil {
		t.Fatal(err)
	}
	p := f.buy(t, 1, 3)
	if p.Kind != model.KindGift || f.status(t, f.code(t, p.ID)) != model.GiftPending {
		t.Fatalf("payment %+v", p)
	}
	// код pending-подарка наружу не отдаётся
	list, _, err := f.s.ListGifts(t.Context(), model.GiftFilter{Limit: 10})
	if err != nil || len(list) != 1 || list[0].Code != "" || list[0].Status != model.GiftPending || list[0].BuyerName != "buyer" {
		t.Fatalf("list = %+v err=%v", list, err)
	}

	if _, err = f.resolve(t.Context(), p.ID, true); err != nil {
		t.Fatal(err)
	}
	code := f.code(t, p.ID)
	if f.status(t, code) != model.GiftReady {
		t.Fatal("not ready")
	}
	if len(f.panel.written(1)) != 0 || f.panel.limits != 0 {
		t.Fatalf("panel touched: writes=%v limits=%d", f.panel.written(1), f.panel.limits)
	}
	if n := f.count(t, `SELECT extra_devices FROM users WHERE telegram_id = 1`); n != 2 {
		t.Fatalf("extra_devices = %d", n)
	}
	items := f.outbox(t)
	if len(items) != 1 || items[0].EventType != payflow.EventGiftResult || items[0].TextKey != "gift.ready" ||
		items[0].GiftCode != code || items[0].Months != 3 || items[0].ChatID != 1 {
		t.Fatalf("outbox = %+v", items)
	}
	// пробный не стал «оплаченным»: счётчик аванса не сброшен
	st, err := f.s.UserStats(t.Context(), 1, 0)
	if err != nil || st.UnpaidAdvances != 1 || st.Confirmed != 1 || st.ConfirmedSum != 500 {
		t.Fatalf("stats = %+v err=%v", st, err)
	}
	// повторное решение
	if _, err = f.resolve(t.Context(), p.ID, true); !errors.Is(err, ErrNotPending) {
		t.Fatalf("repeat: %v", err)
	}
}

func TestGiftPurchaseDeclineCancels(t *testing.T) {
	f := newGiftFixture(t)
	p := f.buy(t, 1, 1)
	if _, err := f.resolve(t.Context(), p.ID, false); err != nil {
		t.Fatal(err)
	}
	code := f.code(t, p.ID)
	if f.status(t, code) != model.GiftCancelled {
		t.Fatal("not cancelled")
	}
	items := f.outbox(t)
	if len(items) != 1 || items[0].TextKey != "gift.declined" || items[0].GiftCode != "" {
		t.Fatalf("outbox = %+v", items)
	}
	if _, _, err := f.redeem(t.Context(), code, 2); !errors.Is(err, ErrGiftUnavailable) {
		t.Fatalf("redeem cancelled: %v", err)
	}
}

// Открытый платёж за подарок не отменяет чужой pending, а сам отменяется новой оплатой подписки или отменой.
func TestGiftPendingRules(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	if _, _, err := f.s.CreatePayment(ctx, newPay(1, f.now), noRollback, nil); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.s.CreatePayment(ctx, model.Payment{TelegramID: 1, Month: 1, Price: 100, PrevExpireAt: f.now, Kind: model.KindGift}, noRollback, nil)
	if !errors.Is(err, ErrPendingExists) {
		t.Fatalf("gift over pending tariff: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM gifts`); n != 0 {
		t.Fatalf("gifts = %d", n)
	}

	// pending-подарок заменяется оплатой подписки; срок читается заново
	f2 := newGiftFixture(t)
	g := f2.buy(t, 1, 2)
	reads := 0
	tariff, cancelled, err := f2.s.CreatePayment(ctx, newPay(1, time.Time{}), noRollback, func(context.Context) (time.Time, error) {
		reads++
		return f2.now.Add(48 * time.Hour), nil
	})
	if err != nil || cancelled == nil || cancelled.ID != g.ID || reads != 1 || !tariff.PrevExpireAt.Equal(f2.now.Add(48*time.Hour)) {
		t.Fatalf("tariff %+v cancelled=%+v reads=%d err=%v", tariff, cancelled, reads, err)
	}
	if f2.status(t, f2.code(t, g.ID)) != model.GiftCancelled {
		t.Fatal("gift must be cancelled")
	}

	// отмена платежа гасит подарок
	f3 := newGiftFixture(t)
	g3 := f3.buy(t, 1, 2)
	if err = f3.s.CancelPayment(ctx, g3.ID); err != nil || f3.status(t, f3.code(t, g3.ID)) != model.GiftCancelled {
		t.Fatalf("cancel payment: %v", err)
	}
}

func TestRedeemExtendsFromMaxOfPanelAndNow(t *testing.T) {
	for name, panelExpire := range map[string]time.Duration{"active": 10 * day, "expired": -5 * day} {
		t.Run(name, func(t *testing.T) {
			f := newGiftFixture(t)
			f.touch(t, 2)
			f.panel.expire[2] = f.now.Add(panelExpire)
			code := f.ready(t, 1, 3)
			for _, it := range f.outbox(t) { // ссылка покупателю нам не нужна
				if err := f.s.DeleteOutbox(t.Context(), it.ID); err != nil {
					t.Fatal(err)
				}
			}

			g, exp, err := f.redeem(t.Context(), code, 2)
			if err != nil {
				t.Fatal(err)
			}
			base := f.now
			if panelExpire > 0 {
				base = f.now.Add(panelExpire)
			}
			want := base.Add(90 * day)
			if !exp.Equal(want) || len(f.panel.written(2)) != 1 || !f.panel.written(2)[0].Equal(want) || g.Status != model.GiftRedeemed {
				t.Fatalf("exp=%v want=%v writes=%v gift=%+v", exp, want, f.panel.written(2), g)
			}
			if f.status(t, code) != model.GiftRedeemed ||
				f.count(t, `SELECT count(*) FROM gifts WHERE code = $1 AND redeemed_by = 2 AND redeemed_at IS NOT NULL`, code) != 1 {
				t.Fatal("gift not marked")
			}
			if f.count(t, `SELECT count(*) FROM grants WHERE telegram_id = 2 AND source = 'gift' AND days = 90 AND by_admin_id IS NULL`) != 1 {
				t.Fatal("grant row missing")
			}
			items := f.outbox(t)
			if len(items) != 2 {
				t.Fatalf("outbox = %+v", items)
			}
			redeemer, buyer := items[0], items[1]
			if redeemer.ChatID != 2 || redeemer.TextKey != "gift.redeemed" || redeemer.Months != 3 || !redeemer.ExpireAt.Equal(want) ||
				buyer.ChatID != 1 || buyer.TextKey != "gift.redeemed_buyer" || buyer.Months != 3 || buyer.GiftCode != "" || buyer.Text != "" {
				t.Fatalf("notices = %+v", items)
			}
			// повтор: ничего больше не выдаётся
			if _, _, err = f.redeem(t.Context(), code, 2); !errors.Is(err, ErrGiftUnavailable) || len(f.panel.written(2)) != 1 {
				t.Fatalf("repeat: %v writes=%v", err, f.panel.written(2))
			}
		})
	}
}

// Покупатель сам активирует свой подарок: разрешено, второго уведомления нет.
func TestRedeemBySelfBuyer(t *testing.T) {
	f := newGiftFixture(t)
	f.touch(t, 1)
	code := f.ready(t, 1, 1)
	if _, _, err := f.redeem(t.Context(), code, 1); err != nil {
		t.Fatal(err)
	}
	var redeemed int
	for _, it := range f.outbox(t) {
		if it.TextKey == "gift.redeemed" || it.TextKey == "gift.redeemed_buyer" {
			redeemed++
		}
	}
	if redeemed != 1 {
		t.Fatalf("notices = %d", redeemed)
	}
}

func TestRedeemExactlyOnceUnderRace(t *testing.T) {
	f := newGiftFixture(t)
	code := f.ready(t, 1, 2)
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { _, _, errs[i] = f.redeem(t.Context(), code, int64(100+i)) })
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrGiftUnavailable):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || f.count(t, `SELECT count(*) FROM grants WHERE source = 'gift'`) != 1 {
		t.Fatalf("won=%d", won)
	}
	total := 0
	for i := range n {
		total += len(f.panel.written(int64(100 + i)))
	}
	if total != 1 {
		t.Fatalf("panel writes = %d", total)
	}
}

// Любая причина отказа даёт одну и ту же ошибку, панель не трогается, подарок остаётся как был.
func TestRedeemRefusalsAreIndistinguishable(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	used := f.ready(t, 1, 1)
	if _, _, err := f.redeem(ctx, used, 5); err != nil {
		t.Fatal(err)
	}
	admin, err := f.s.CreateAdminGifts(ctx, 1, 2, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelGift(ctx, admin[0].ID); err != nil {
		t.Fatal(err)
	}
	pending := f.code(t, f.buy(t, 7, 1).ID)
	f.touch(t, 6)
	if err = f.s.SetUserBlocked(ctx, 6, true, f.now); err != nil {
		t.Fatal(err)
	}
	writes := len(f.panel.written(6)) + len(f.panel.written(8))
	cases := map[string]struct {
		code string
		by   int64
	}{
		"unknown":   {"AAAAAAAAAAAAAAAAAAAAAA", 8},
		"used":      {used, 8},
		"cancelled": {admin[0].Code, 8},
		"pending":   {pending, 8},
		"blocked":   {admin[1].Code, 6},
	}
	for name, tc := range cases {
		_, _, err := f.redeem(ctx, tc.code, tc.by)
		if err != ErrGiftUnavailable { //nolint:errorlint // ровно тот же sentinel, без обёрток: ответы не различаются
			t.Errorf("%s: err = %#v", name, err)
		}
	}
	if len(f.panel.written(6))+len(f.panel.written(8)) != writes {
		t.Fatal("panel touched on refusal")
	}
	if f.status(t, admin[1].Code) != model.GiftReady || f.status(t, pending) != model.GiftPending {
		t.Fatal("refusal changed gift")
	}
	// после снятия блокировки тот же подарок работает
	if err = f.s.SetUserBlocked(ctx, 6, false, f.now); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.redeem(ctx, admin[1].Code, 6); err != nil {
		t.Fatalf("after unblock: %v", err)
	}
}

// Новый юзер: строки в users ещё нет; Prepare (заведение в панели) зовётся внутри транзакции.
func TestRedeemNewUserRunsPrepare(t *testing.T) {
	f := newGiftFixture(t)
	code := f.ready(t, 1, 1)
	prepared := 0
	_, exp, err := payflow.RedeemGift(t.Context(), f.s, f.panel, payflow.GiftRedeem{
		Code: code, TelegramID: 42, At: f.now, Prepare: func(context.Context) error { prepared++; return nil },
	})
	if err != nil || prepared != 1 || !exp.Equal(f.now.Add(30*day)) {
		t.Fatalf("exp=%v prepared=%d err=%v", exp, prepared, err)
	}
	// сбой Prepare откатывает активацию
	code2 := f.ready(t, 1, 1)
	_, _, err = payflow.RedeemGift(t.Context(), f.s, f.panel, payflow.GiftRedeem{
		Code: code2, TelegramID: 43, At: f.now, Prepare: func(context.Context) error { return errors.New("create failed") },
	})
	if !errors.Is(err, payflow.ErrPanel) || f.status(t, code2) != model.GiftReady {
		t.Fatalf("err=%v status=%v", err, f.status(t, code2))
	}
}

func TestRedeemPanelFailureRollsBackAndRetries(t *testing.T) {
	f := newGiftFixture(t)
	f.touch(t, 2)
	f.panel.expire[2] = f.now.Add(5 * day)
	if _, _, err := f.s.CreatePayment(t.Context(), newPay(2, f.now.Add(5*day)), noRollback, nil); err != nil {
		t.Fatal(err)
	}
	code := f.ready(t, 1, 1)
	for _, it := range f.outbox(t) {
		_ = f.s.DeleteOutbox(t.Context(), it.ID)
	}

	f.panel.setFail(2, errors.New("panel down"))
	if _, _, err := f.redeem(t.Context(), code, 2); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	prev := pendingPrev(t, f.s, 2)
	if f.status(t, code) != model.GiftReady || f.count(t, `SELECT count(*) FROM grants WHERE source = 'gift'`) != 0 ||
		len(f.outbox(t)) != 0 || !prev.Equal(f.now.Add(5*day)) {
		t.Fatalf("not rolled back: status=%v prev=%v", f.status(t, code), prev)
	}

	f.panel.setFail(2, nil)
	_, exp, err := f.redeem(t.Context(), code, 2)
	want := f.now.Add(35 * day)
	if err != nil || !exp.Equal(want) {
		t.Fatalf("retry: %v %v", exp, err)
	}
	// база pending-тарифа сдвинута на те же 30 суток
	if got := pendingPrev(t, f.s, 2); !got.Equal(want) {
		t.Fatalf("pending prev = %v want %v", got, want)
	}
}

type failCommitGift struct {
	*Store
	err error
}

func (f failCommitGift) RedeemGift(ctx context.Context, code string, id int64, now time.Time, apply func(model.Gift) ([]model.OutboxMessage, error)) (model.Gift, error) {
	g, err := f.Store.RedeemGift(ctx, code, id, now, apply)
	if err != nil {
		return g, err
	}
	return g, f.err
}

// Сбой коммита после записи в панель: прежний срок возвращается (пока он ещё наш).
func TestRedeemCommitFailureRestoresExpire(t *testing.T) {
	f := newGiftFixture(t)
	f.touch(t, 2)
	old := f.now.Add(3 * day)
	f.panel.expire[2] = old
	code := f.ready(t, 1, 1)
	boom := errors.New("commit failed")
	_, _, err := payflow.RedeemGift(t.Context(), failCommitGift{f.s, boom}, f.panel, payflow.GiftRedeem{Code: code, TelegramID: 2, At: f.now})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := f.panel.GetUser(t.Context(), 2); !got.ExpireAt.Equal(old) {
		t.Fatalf("panel expire = %v want %v", got.ExpireAt, old)
	}
}

// Подтверждённый подарок — не оплата тарифа: рефералка, «только новым», пробный, счётчик авансов.
func TestGiftPaymentIsNotATariff(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	if _, err := f.s.RecordReferral(ctx, model.UserSeen{TelegramID: 20, Username: "friend", At: f.now}, 10); err != nil {
		t.Fatal(err)
	}
	seedPayment(t, f.s, 20, "declined", true) // неоплаченный аванс до подарка
	p := f.buy(t, 20, 1)
	res, err := f.resolve(ctx, p.ID, true)
	if err != nil || res.ReferralPending {
		t.Fatalf("referral: pending=%v err=%v", res.ReferralPending, err)
	}
	if f.count(t, `SELECT count(*) FROM referral_rewards`) != 0 || f.count(t, `SELECT count(*) FROM grants`) != 0 {
		t.Fatal("referral reward for a gift")
	}
	if has, err := f.s.HasConfirmed(ctx, 20); err != nil || has {
		t.Fatalf("HasConfirmed = %v err=%v (only-new promo must still see the user as new)", has, err)
	}
	st, err := f.s.UserStats(ctx, 20, 0)
	if err != nil || st.UnpaidAdvances != 1 {
		t.Fatalf("advance counter reset by a gift: %+v err=%v", st, err)
	}
	if ok, err := f.s.TrialAllowed(ctx, 20, 3); err != nil || !ok {
		t.Fatalf("trial must stay available after buying a gift: %v %v", ok, err)
	}
	users, _, err := f.s.ListUsers(ctx, model.UserFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.TelegramID == 20 && u.TariffMonths != nil {
			t.Fatalf("tariff field set by a gift: %d", *u.TariffMonths)
		}
	}
}

func TestAdminGiftsCreateCancel(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	gs, err := f.s.CreateAdminGifts(ctx, 6, 5, f.now)
	if err != nil || len(gs) != 5 {
		t.Fatalf("gifts = %v err=%v", gs, err)
	}
	seen := map[string]bool{}
	for _, g := range gs {
		if g.Status != model.GiftReady || !g.ByAdmin || g.Months != 6 || len(g.Code) != 22 || seen[g.Code] || g.BuyerID != nil {
			t.Fatalf("gift = %+v", g)
		}
		seen[g.Code] = true
	}
	if err = f.s.CancelGift(ctx, gs[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelGift(ctx, gs[0].ID); !errors.Is(err, ErrGiftNotActive) {
		t.Fatalf("cancel twice: %v", err)
	}
	if _, _, err = f.redeem(ctx, gs[1].Code, 9); err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelGift(ctx, gs[1].ID); !errors.Is(err, ErrGiftNotActive) {
		t.Fatalf("cancel redeemed: %v", err)
	}
	if err = f.s.CancelGift(ctx, 99999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel unknown: %v", err)
	}
	// вкладки и страницы
	for st, want := range map[model.GiftStatus]int{"": 5, model.GiftReady: 3, model.GiftRedeemed: 1, model.GiftCancelled: 1} {
		list, total, err := f.s.ListGifts(ctx, model.GiftFilter{Status: st, Limit: 2})
		if err != nil || total != want || len(list) > 2 {
			t.Fatalf("status %q: total=%d len=%d err=%v", st, total, len(list), err)
		}
	}
	list, _, _ := f.s.ListGifts(ctx, model.GiftFilter{Status: model.GiftRedeemed, Limit: 10})
	if len(list) != 1 || list[0].RedeemedBy == nil || *list[0].RedeemedBy != 9 || list[0].RedeemedAt == nil {
		t.Fatalf("redeemed list = %+v", list)
	}
}

// Подтверждение, активация, выдача дней и оплата подписки одних и тех же юзеров вперемешку: ни дедлока, ни лишних ошибок.
func TestGiftNoDeadlockMix(t *testing.T) {
	f := newGiftFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	users := []int64{1, 2, 3}
	for _, id := range users {
		f.touch(t, id)
	}
	ok := func(err error) bool {
		return err == nil || errors.Is(err, ErrNotPending) || errors.Is(err, ErrPendingExists) ||
			errors.Is(err, ErrGiftUnavailable) || errors.Is(err, ErrNotFound)
	}
	for round := range 12 {
		a, b := users[round%3], users[(round+1)%3]
		codes := make([]string, 0, 2)
		for range 2 {
			c, err := f.s.CreateAdminGifts(ctx, 1, 1, f.now)
			if err != nil {
				t.Fatal(err)
			}
			codes = append(codes, c[0].Code)
		}
		gp, _, gerr := f.s.CreatePayment(ctx, model.Payment{TelegramID: a, Month: 1, Price: 100, PrevExpireAt: f.now, Kind: model.KindGift}, noRollback, nil)
		if gerr != nil && !ok(gerr) {
			t.Fatal(gerr)
		}
		var wg sync.WaitGroup
		run := func(fn func() error) {
			wg.Go(func() {
				if err := fn(); !ok(err) {
					t.Errorf("round %d: %v", round, err)
				}
			})
		}
		run(func() error { _, err := f.resolve(ctx, gp.ID, round%2 == 0); return err })
		run(func() error { _, _, err := f.redeem(ctx, codes[0], a); return err })
		run(func() error { _, _, err := f.redeem(ctx, codes[1], b); return err })
		run(func() error {
			_, err := payflow.GrantDays(ctx, f.s, f.panel, payflow.GrantRequest{TelegramID: a, Days: 3, At: f.now})
			return err
		})
		run(func() error {
			_, _, err := f.s.CreatePayment(ctx, newPay(b, time.Time{}), noRollback, func(context.Context) (time.Time, error) { return f.now, nil })
			return err
		})
		wg.Wait()
		if ctx.Err() != nil {
			t.Fatal("deadlock or timeout")
		}
	}
}

// Блокировка запрещает бесплатное, а не платное: заблокированный покупает и получает ссылку, но сам по ссылке не активирует.
func TestBlockedBuyerCanPayForGift(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	f.touch(t, 1)
	if err := f.s.SetUserBlocked(ctx, 1, true, f.now); err != nil {
		t.Fatal(err)
	}
	code := f.ready(t, 1, 2)
	if _, _, err := f.redeem(ctx, code, 1); !errors.Is(err, ErrGiftUnavailable) {
		t.Fatalf("blocked buyer redeems his own gift: %v", err)
	}
	if _, _, err := f.redeem(ctx, code, 2); err != nil {
		t.Fatalf("a friend redeems: %v", err)
	}
}

func TestDeleteGiftPaths(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	gs, _ := f.s.CreateAdminGifts(ctx, 1, 3, f.now) // 0: ready, удаляем; 1: cancelled; 2: redeemed
	if err := f.s.CancelGift(ctx, gs[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.redeem(ctx, gs[2].Code, 9); err != nil {
		t.Fatal(err)
	}
	// подарок админа стирается физически (ready — одним шагом, cancelled — как есть); повтор — 404
	for _, g := range gs[:2] {
		if err := f.s.DeleteGift(ctx, g.ID); err != nil {
			t.Fatal(err)
		}
		if n := f.count(t, `SELECT count(*) FROM gifts WHERE id = $1`, g.ID); n != 0 {
			t.Fatalf("admin gift %d stays", g.ID)
		}
		if err := f.s.DeleteGift(ctx, g.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("repeat: %v", err)
		}
	}
	// использованный — история
	if err := f.s.DeleteGift(ctx, gs[2].ID); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("redeemed: %v", err)
	}

	// купленный: ожидает оплаты нельзя; оплаченный ready — мягко, платёж и деньги на месте
	f.touch(t, 1)
	pend := f.buy(t, 1, 2)
	var pendID int64
	_ = f.s.pool.QueryRow(ctx, `SELECT id FROM gifts WHERE payment_id = $1`, pend.ID).Scan(&pendID)
	if err := f.s.DeleteGift(ctx, pendID); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("pending: %v", err)
	}
	if _, err := f.resolve(ctx, pend.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteGift(ctx, pendID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteGift(ctx, pendID); err != nil {
		t.Fatalf("repeat soft: %v", err)
	}
	code := f.code(t, pend.ID)
	if f.status(t, code) != model.GiftCancelled || f.count(t, `SELECT count(*) FROM gifts WHERE id = $1 AND deleted_at IS NOT NULL`, pendID) != 1 ||
		f.count(t, `SELECT count(*) FROM payments WHERE id = $1 AND status = 'confirmed'`, pend.ID) != 1 {
		t.Fatal("bought gift must stay as a cancelled row")
	}
	// активация по ссылке удалённого подарка — тот же нейтральный отказ
	if _, _, err := f.redeem(ctx, code, 10); !errors.Is(err, ErrGiftUnavailable) {
		t.Fatalf("redeem deleted: %v", err)
	}
	// из списков исчез, в счёт не входит
	for _, st := range []model.GiftStatus{"", model.GiftCancelled} {
		list, total, err := f.s.ListGifts(ctx, model.GiftFilter{Status: st, Limit: 10})
		for _, g := range list {
			if g.ID == pendID {
				t.Fatalf("deleted in list %q", st)
			}
		}
		if err != nil || total != len(list) {
			t.Fatalf("status %q: total=%d len=%d err=%v", st, total, len(list), err)
		}
	}
}

// Удаление и активация готового подарка идут параллельно: либо активировали (удалить нельзя), либо удалили (активация отказана).
func TestDeleteGiftRacesRedeem(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	for i := range 15 {
		g, _ := f.s.CreateAdminGifts(ctx, 1, 1, f.now)
		var delErr, redErr error
		var wg sync.WaitGroup
		wg.Go(func() { delErr = f.s.DeleteGift(ctx, g[0].ID) })
		wg.Go(func() { _, _, redErr = f.redeem(ctx, g[0].Code, int64(500+i)) })
		wg.Wait()
		switch {
		case delErr == nil && errors.Is(redErr, ErrGiftUnavailable):
			if f.count(t, `SELECT count(*) FROM gifts WHERE id = $1`, g[0].ID) != 0 {
				t.Fatalf("iter %d: deleted but row stays", i)
			}
		case errors.Is(delErr, ErrNotDeletable) && redErr == nil:
			if f.count(t, `SELECT count(*) FROM gifts WHERE id = $1 AND status = 'redeemed'`, g[0].ID) != 1 {
				t.Fatalf("iter %d: redeem lost", i)
			}
		default:
			t.Fatalf("iter %d: delete=%v redeem=%v", i, delErr, redErr)
		}
	}
}

// Отзыв оплаченного подарка (кнопкой «Отозвать» или «Удалить» у готового) сообщает покупателю в той же транзакции;
// подарок админа без покупателя, повторный отзыв и удаление уже отозванного ничего не шлют.
func TestGiftRevokeNotifiesBuyer(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	f.touch(t, 1)
	cancelled := func() []model.OutboxItem {
		var out []model.OutboxItem
		for _, it := range f.outbox(t) {
			if it.TextKey == "gift.cancelled" {
				out = append(out, it)
			}
		}
		return out
	}

	code := f.ready(t, 1, 3)
	var id int64
	if err := f.s.pool.QueryRow(ctx, `SELECT id FROM gifts WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := f.s.CancelGift(ctx, id); err != nil {
		t.Fatal(err)
	}
	n := cancelled()
	if len(n) != 1 || n[0].ChatID != 1 || n[0].Months != 3 || n[0].EventType != "gift_cancelled" {
		t.Fatalf("notice = %+v", n)
	}
	if err := f.s.CancelGift(ctx, id); !errors.Is(err, ErrGiftNotActive) || len(cancelled()) != 1 {
		t.Fatalf("repeat cancel: %v, notices %d", err, len(cancelled()))
	}
	if err := f.s.DeleteGift(ctx, id); err != nil || len(cancelled()) != 1 { // уже отозван: молча
		t.Fatalf("delete of a cancelled gift: %v, notices %d", err, len(cancelled()))
	}

	// «Удалить» готовый купленный: отзыв и удаление одним шагом, покупатель узнаёт
	code2 := f.ready(t, 1, 6)
	var id2 int64
	if err := f.s.pool.QueryRow(ctx, `SELECT id FROM gifts WHERE code = $1`, code2).Scan(&id2); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DeleteGift(ctx, id2); err != nil {
		t.Fatal(err)
	}
	if n := cancelled(); len(n) != 2 || n[1].Months != 6 {
		t.Fatalf("delete of a ready gift: %+v", n)
	}

	// подарок админа: покупателя нет, слать некому
	admin, err := f.s.CreateAdminGifts(ctx, 1, 2, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelGift(ctx, admin[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DeleteGift(ctx, admin[1].ID); err != nil {
		t.Fatal(err)
	}
	if len(cancelled()) != 2 {
		t.Fatalf("admin gifts must not notify: %d", len(cancelled()))
	}
}

// Пачка подарков с ключом создаётся один раз; повтор (в том числе параллельный) возвращает её же.
func TestCreateAdminGiftsOnce(t *testing.T) {
	f := newGiftFixture(t)
	ctx := t.Context()
	key := uuid.New()
	first, created, err := f.s.CreateAdminGiftsOnce(ctx, key, 3, 2, f.now)
	if err != nil || !created || len(first) != 2 {
		t.Fatalf("first: %v created=%v n=%d", err, created, len(first))
	}
	again, created, err := f.s.CreateAdminGiftsOnce(ctx, key, 12, 5, f.now) // месяцы и число из повтора игнорируются
	if err != nil || created || len(again) != 2 || again[0].Code != first[0].Code || again[1].Code != first[1].Code || again[0].Months != 3 {
		t.Fatalf("repeat: %v created=%v %+v", err, created, again)
	}
	// без ключа — всегда новая пачка
	if _, created, err = f.s.CreateAdminGiftsOnce(ctx, uuid.Nil, 1, 1, f.now); err != nil || !created {
		t.Fatalf("nil key: %v %v", created, err)
	}
	// гонка: восемь одинаковых запросов дают одну пачку
	race := uuid.New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := f.s.CreateAdminGiftsOnce(ctx, race, 1, 3, f.now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := f.count(t, `SELECT count(*) FROM gifts WHERE batch_key = $1`, race); n != 3 {
		t.Fatalf("race: %d gifts for one key", n)
	}
}
