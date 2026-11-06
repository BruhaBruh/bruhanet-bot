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
	"github.com/bruhabruh/bruhanet/core/promo"
)

// statePanel — панель с состоянием: чтение срока и запись с задержкой (транзакция держит блокировки).
type statePanel struct {
	mu     sync.Mutex
	expire time.Time
	err    error
	writes []time.Time
}

func (p *statePanel) cur() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.expire
}

func (p *statePanel) UpdateUser(_ context.Context, _ int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	time.Sleep(100 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	p.expire = expireAt
	p.writes = append(p.writes, expireAt)
	return &api.UserItemInfo{ExpireAt: expireAt}, nil
}

func (p *statePanel) CreateUser(context.Context, int64, string, uuid.UUID) (*api.UserItemInfo, error) {
	return &api.UserItemInfo{ExpireAt: p.cur()}, nil
}

func (p *statePanel) GetUser(context.Context, int64) (*api.UserItemInfo, error) {
	return &api.UserItemInfo{ExpireAt: p.cur()}, nil
}

func pendingPrev(t *testing.T, s *Store, id int64) time.Time {
	t.Helper()
	p, err := s.PendingPayment(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p.PrevExpireAt
}

func TestGrantDays(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
	if _, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback); err != nil {
		t.Fatal(err)
	}
	panel := &statePanel{expire: prev}
	req := payflow.GrantRequest{TelegramID: 1, Username: "u", Days: 7, AdminID: 99, At: now}

	// ошибка панели: ничего не записано, pending не сдвинут
	panel.err = errors.New("panel down")
	if _, err := payflow.GrantDays(ctx, s, panel, req); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if gs, _ := s.UserGrants(ctx, 1, 20); len(gs) != 0 || !pendingPrev(t, s, 1).Equal(prev) || len(outboxTypes(t, s)) != 0 {
		t.Fatalf("panel error must roll back: grants=%v prev=%v", gs, pendingPrev(t, s, 1))
	}

	panel.err = nil
	got, err := payflow.GrantDays(ctx, s, panel, req)
	if err != nil {
		t.Fatal(err)
	}
	want := prev.Add(7 * 24 * time.Hour)
	if !got.Equal(want) || !panel.cur().Equal(want) {
		t.Fatalf("expire = %v, panel = %v, want %v", got, panel.cur(), want)
	}
	// база pending-платежа сдвинута на те же дни: confirm/decline не затрёт бонус
	if !pendingPrev(t, s, 1).Equal(want) {
		t.Fatalf("pending prev = %v, want %v", pendingPrev(t, s, 1), want)
	}
	gs, err := s.UserGrants(ctx, 1, 20)
	if err != nil || len(gs) != 1 || gs[0].Days != 7 {
		t.Fatalf("grants = %+v err=%v", gs, err)
	}
	items, _ := s.OutboxBatch(ctx, 10)
	if len(items) != 1 || items[0].EventType != payflow.EventGrant || items[0].TextKey != "grant.days" ||
		items[0].Days != 7 || !items[0].ExpireAt.Equal(want) || items[0].ChatID != 1 {
		t.Fatalf("outbox = %+v", items)
	}

	// нет юзера
	if _, err = payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 404, Days: 1, At: now}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	// вне диапазона
	for _, d := range []int{0, -1, 366} {
		if _, err = payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: d, At: now}); !errors.Is(err, payflow.ErrGrantDays) {
			t.Fatalf("days %d: %v", d, err)
		}
	}
}

// Срок уже в прошлом: выдача идёт от now.
func TestGrantDaysFromNowWhenExpired(t *testing.T) {
	s := testStore(t)
	now := time.Now().Truncate(time.Microsecond)
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 3, At: now}); err != nil {
		t.Fatal(err)
	}
	panel := &statePanel{expire: now.Add(-90 * 24 * time.Hour)}
	got, err := payflow.GrantDays(t.Context(), s, panel, payflow.GrantRequest{TelegramID: 3, Days: 10, At: now})
	if err != nil || !got.Equal(now.Add(10*24*time.Hour)) {
		t.Fatalf("got %v err=%v", got, err)
	}
}

// Сбой коммита после записи в панель: срок возвращается; админ поменял срок сам — не затираем.
type failCommitStore struct {
	*Store
	err error
}

func (f failCommitStore) GrantDays(ctx context.Context, id, admin int64, days int, now time.Time, apply func() ([]model.OutboxMessage, error)) error {
	if err := f.Store.GrantDays(ctx, id, admin, days, now, apply); err != nil {
		return err
	}
	return f.err
}

func TestGrantDaysCommitFailureRestoresExpire(t *testing.T) {
	s := testStore(t)
	now := time.Now().Truncate(time.Microsecond)
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 3, At: now}); err != nil {
		t.Fatal(err)
	}
	old := now.Add(24 * time.Hour)
	panel := &statePanel{expire: old}
	boom := errors.New("commit failed")
	_, err := payflow.GrantDays(t.Context(), failCommitStore{s, boom}, panel, payflow.GrantRequest{TelegramID: 3, Days: 5, At: now})
	if !errors.Is(err, boom) || !panel.cur().Equal(old) {
		t.Fatalf("err=%v panel=%v want %v", err, panel.cur(), old)
	}
}

// Подтверждение платежа и выдача дней одновременно: в любом порядке итог — prev + дни + месяц.
func TestGrantDaysRaceWithConfirm(t *testing.T) {
	for _, tc := range []struct {
		name         string
		grantFirst   bool
		grantDelay   time.Duration
		confirmDelay time.Duration
	}{
		{"confirm first", false, 40 * time.Millisecond, 0},
		{"grant first", true, 0, 40 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			ctx := t.Context()
			now := time.Now().Truncate(time.Microsecond)
			prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
			p, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback)
			if err != nil {
				t.Fatal(err)
			}
			panel := &statePanel{expire: prev}
			var wg sync.WaitGroup
			var gerr, cerr error
			wg.Go(func() {
				time.Sleep(tc.grantDelay)
				_, gerr = payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: 7, At: now})
			})
			wg.Go(func() {
				time.Sleep(tc.confirmDelay)
				_, cerr = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID, Approve: true, At: now})
			})
			wg.Wait()
			if gerr != nil || cerr != nil {
				t.Fatalf("grant=%v confirm=%v", gerr, cerr)
			}
			want := prev.Add(7*24*time.Hour + payflow.Month)
			if !panel.cur().Equal(want) {
				t.Fatalf("panel = %v, want %v (writes %v)", panel.cur(), want, panel.writes)
			}
		})
	}
}

// Дни по промокоду и выдача админом одновременно: оба сдвигают pending, ни один не теряется.
func TestGrantDaysRaceWithPromoDays(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
	if _, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback); err != nil {
		t.Fatal(err)
	}
	pr := mustPromo(t, s, model.Promo{Code: "DAYS3", Kind: promo.KindDays, Value: 3})
	panel := &statePanel{expire: prev}
	var wg sync.WaitGroup
	var gerr, perr error
	wg.Go(func() {
		_, gerr = payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: 7, At: now})
	})
	wg.Go(func() {
		perr = s.ActivateDaysPromo(ctx, pr.ID, 1, now, func(model.Promo) error { time.Sleep(100 * time.Millisecond); return nil })
	})
	wg.Wait()
	if gerr != nil || perr != nil {
		t.Fatalf("grant=%v promo=%v", gerr, perr)
	}
	if want := prev.Add(10 * 24 * time.Hour); !pendingPrev(t, s, 1).Equal(want) {
		t.Fatalf("pending prev = %v, want %v", pendingPrev(t, s, 1), want)
	}
}

func TestSetUserBlocked(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if err := s.SetUserBlocked(ctx, 404, true, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	t1 := time.Now().Truncate(time.Microsecond)
	for _, at := range []time.Time{t1, t1.Add(time.Hour)} { // повтор идемпотентен, время первой блокировки сохраняется
		if err := s.SetUserBlocked(ctx, 1, true, at); err != nil {
			t.Fatal(err)
		}
	}
	u, _ := s.UserByID(ctx, 1)
	if u.BlockedAt == nil || !u.BlockedAt.Equal(t1) {
		t.Fatalf("blocked_at = %v, want %v", u.BlockedAt, t1)
	}
	if b, _ := s.UserBlocked(ctx, 1); !b {
		t.Fatal("UserBlocked = false")
	}
	for range 2 {
		if err := s.SetUserBlocked(ctx, 1, false, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if u, _ = s.UserByID(ctx, 1); u.BlockedAt != nil {
		t.Fatalf("unblock: %v", u.BlockedAt)
	}
}

// Ручная блокировка закрывает каждую бесплатную выдачу и не мешает платной оплате.
func TestBlockedUserGetsNothingFree(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
	pay, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	discount := mustPromo(t, s, model.Promo{Code: "OFF50", Kind: promo.KindPercent, Value: 50})
	free := mustPromo(t, s, model.Promo{Code: "FREE", Kind: promo.KindPercent, Value: 100})
	days := mustPromo(t, s, model.Promo{Code: "DAYS", Kind: promo.KindDays, Value: 5})

	// 0 ₽-платёж создан до блокировки; блокировка ставится до подтверждения
	zero := newPay(2, prev)
	zero.Price, zero.PromoID, zero.BasePrice = 0, &free.ID, ptr(150)
	zeroPay, _, err := s.CreatePayment(ctx, zero, noRollback)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		if err = s.SetUserBlocked(ctx, id, true, now); err != nil {
			t.Fatal(err)
		}
	}

	called := false
	// аванс: ErrBlocked, флаг не стоит, панель не тронута
	if err = s.GrantAdvance(ctx, pay.ID, func(model.Payment) { called = true }); !errors.Is(err, ErrBlocked) || called {
		t.Fatalf("advance: err=%v called=%v", err, called)
	}
	if got, _ := s.PaymentByID(ctx, pay.ID); got.AdvanceGranted {
		t.Fatal("advance flag set for blocked user")
	}
	// дни по промокоду
	if err = s.ActivateDaysPromo(ctx, days.ID, 1, now, func(model.Promo) error { called = true; return nil }); !errors.Is(err, promo.ErrBlocked) || called {
		t.Fatalf("days promo: err=%v called=%v", err, called)
	}
	if !pendingPrev(t, s, 1).Equal(prev) {
		t.Fatal("blocked days promo shifted pending")
	}
	// скидка при создании платежа
	np := newPay(1, prev)
	np.PromoID, np.Price = &discount.ID, 75
	if _, _, err = s.CreatePayment(ctx, np, noRollback); !errors.Is(err, promo.ErrBlocked) {
		t.Fatalf("discount: %v", err)
	}
	if got, _ := s.PendingPayment(ctx, 1); got.ID != pay.ID {
		t.Fatalf("blocked discount replaced pending: %+v", got)
	}
	// 0 ₽: подтверждение отклонено под блокировкой кода, платёж остался pending, активации нет
	_, err = s.ResolvePayment(ctx, zeroPay.ID, model.PaymentConfirmed, now, func(model.Payment) ([]model.OutboxMessage, error) {
		called = true
		return nil, nil
	})
	if !errors.Is(err, promo.ErrBlocked) || called {
		t.Fatalf("price-0: err=%v called=%v", err, called)
	}
	if got, _ := s.PaymentByID(ctx, zeroPay.ID); got.Status != model.PaymentPending {
		t.Fatalf("status = %s", got.Status)
	}
	if used, _, _ := s.PromoUsage(ctx, free.ID, 2); used != 0 {
		t.Fatalf("activations = %d", used)
	}

	// обычная оплата доступна: подтверждение платного платежа без промокода проходит
	if _, err = s.ResolvePayment(ctx, pay.ID, model.PaymentConfirmed, now, func(model.Payment) ([]model.OutboxMessage, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("paid confirm for blocked user: %v", err)
	}

	// после снятия блокировки всё снова работает
	if err = s.SetUserBlocked(ctx, 1, false, now); err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateDaysPromo(ctx, days.ID, 1, now, func(model.Promo) error { return nil }); err != nil {
		t.Fatalf("after unblock: %v", err)
	}
}

// Админская выдача дней работает и для заблокированного вручную юзера (решение админа).
func TestGrantDaysWorksForBlockedUser(t *testing.T) {
	s := testStore(t)
	now := time.Now().Truncate(time.Microsecond)
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 5, At: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserBlocked(t.Context(), 5, true, now); err != nil {
		t.Fatal(err)
	}
	panel := &statePanel{expire: now}
	if _, err := payflow.GrantDays(t.Context(), s, panel, payflow.GrantRequest{TelegramID: 5, Days: 3, At: now}); err != nil {
		t.Fatalf("admin grant for blocked: %v", err)
	}
}

func TestUserStatsBlocked(t *testing.T) {
	s := testStore(t)
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 1, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.UserStats(t.Context(), 1, 0); st.Blocked {
		t.Fatal("blocked before block")
	}
	if err := s.SetUserBlocked(t.Context(), 1, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st, err := s.UserStats(t.Context(), 1, 0); err != nil || !st.Blocked {
		t.Fatalf("stats = %+v err=%v", st, err)
	}
}
