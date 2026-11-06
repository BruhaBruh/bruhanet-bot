package store

import (
	"context"
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/promo"
)

const day = 24 * time.Hour

// Окна гонки форсируются задержками: операция A держит замок юзера ~100 мс, B стартует через 30 мс.
func termFixture(t *testing.T) (s *Store, panel *statePanel, now, prev time.Time) {
	t.Helper()
	s = testStore(t)
	now = time.Now().Truncate(time.Microsecond)
	prev = now.Add(48 * time.Hour).Truncate(time.Microsecond)
	panel = &statePanel{expire: prev}
	return s, panel, now, prev
}

// readPanel — readPrev бота: срок из панели, затем «долгий» ответ (держит замок).
func readPanel(panel *statePanel, hold time.Duration) func(context.Context) (time.Time, error) {
	return func(context.Context) (time.Time, error) {
		v := panel.cur()
		time.Sleep(hold)
		return v, nil
	}
}

func raceTwo(first, second func()) {
	var wg sync.WaitGroup
	wg.Go(first)
	time.Sleep(30 * time.Millisecond)
	wg.Go(second)
	wg.Wait()
}

// confirmedTerm подтверждает pending юзера 1 и возвращает базу, от которой считается срок.
func confirmedTerm(t *testing.T, s *Store) time.Time {
	t.Helper()
	p, err := s.PendingPayment(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	var base time.Time
	if _, err = s.ResolvePayment(t.Context(), p.ID, model.PaymentConfirmed, time.Now(), func(p model.Payment) ([]model.OutboxMessage, error) {
		base = p.PrevExpireAt
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	return base.Add(30 * day)
}

func TestTermGrantVsCreatePayment(t *testing.T) {
	for _, grantFirst := range []bool{true, false} {
		s, panel, now, prev := termFixture(t)
		ctx := t.Context()
		if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, At: now}); err != nil {
			t.Fatal(err)
		}
		grant := func() {
			if _, err := payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: 7, At: now}); err != nil {
				t.Error(err)
			}
		}
		create := func(hold time.Duration) func() {
			return func() {
				if _, _, err := s.CreatePayment(ctx, newPay(1, time.Time{}), noRollback, readPanel(panel, hold)); err != nil {
					t.Error(err)
				}
			}
		}
		if grantFirst {
			raceTwo(grant, create(0))
		} else {
			raceTwo(create(100*time.Millisecond), grant)
		}
		if want := prev.Add(7*day + 30*day); !confirmedTerm(t, s).Equal(want) || !panel.cur().Equal(prev.Add(7*day)) {
			t.Fatalf("grantFirst=%v: term lost the grant (panel %v)", grantFirst, panel.cur())
		}
	}
}

// Есть pending: CreatePayment отменяет его (rollback долгий) и наследует базу, выдача дней не теряется.
func TestTermGrantVsCreatePaymentCancellingPending(t *testing.T) {
	for _, grantFirst := range []bool{true, false} {
		s, panel, now, prev := termFixture(t)
		ctx := t.Context()
		if _, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback, nil); err != nil {
			t.Fatal(err)
		}
		grant := func() {
			if _, err := payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: 7, At: now}); err != nil {
				t.Error(err)
			}
		}
		create := func() {
			slow := func(model.Payment) error { time.Sleep(100 * time.Millisecond); return nil }
			if _, _, err := s.CreatePayment(ctx, newPay(1, time.Time{}), slow, readPanel(panel, 0)); err != nil {
				t.Error(err)
			}
		}
		if grantFirst {
			raceTwo(grant, create)
		} else {
			raceTwo(create, grant)
		}
		if got := pendingPrev(t, s, 1); !got.Equal(prev.Add(7 * day)) {
			t.Fatalf("grantFirst=%v: new pending prev = %v, want %v", grantFirst, got, prev.Add(7*day))
		}
	}
}

func TestTermDaysPromoVsCreatePayment(t *testing.T) {
	for _, withPending := range []bool{false, true} {
		for _, promoFirst := range []bool{true, false} {
			s, panel, now, prev := termFixture(t)
			ctx := t.Context()
			pr := mustPromo(t, s, model.Promo{Code: "D3", Kind: promo.KindDays, Value: 3})
			if withPending {
				if _, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback, nil); err != nil {
					t.Fatal(err)
				}
			} else if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, At: now}); err != nil {
				t.Fatal(err)
			}
			act := func() {
				err := s.ActivateDaysPromo(ctx, pr.ID, 1, now, func(model.Promo) error {
					time.Sleep(100 * time.Millisecond)
					_, err := panel.UpdateUser(ctx, 1, "u", panel.cur().Add(3*day), [16]byte{})
					return err
				})
				if err != nil {
					t.Error(err)
				}
			}
			hold := time.Duration(0)
			rb := noRollback
			if !promoFirst {
				hold = 100 * time.Millisecond
				rb = func(model.Payment) error { time.Sleep(100 * time.Millisecond); return nil }
			}
			create := func() {
				if _, _, err := s.CreatePayment(ctx, newPay(1, time.Time{}), rb, readPanel(panel, hold)); err != nil {
					t.Error(err)
				}
			}
			if promoFirst {
				raceTwo(act, create)
			} else {
				raceTwo(create, act)
			}
			if want := prev.Add(3*day + 30*day); !confirmedTerm(t, s).Equal(want) {
				t.Fatalf("pending=%v promoFirst=%v: promo days lost", withPending, promoFirst)
			}
		}
	}
}

// 50 случайных перемежений пяти операций на одного юзера: ни дедлоков, ни зависаний.
func TestTermNoDeadlock(t *testing.T) {
	s := testStore(t)
	now := time.Now().Truncate(time.Microsecond)
	pr := mustPromo(t, s, model.Promo{Code: "DL", Kind: promo.KindDays, Value: 1})
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 50 {
		id := int64(1000 + i)
		panel := &statePanel{expire: now.Add(10 * day), fast: true}
		if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: id, At: now}); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		pend := func() int64 {
			p, err := s.PendingPayment(ctx, id)
			if err != nil {
				return 0
			}
			return p.ID
		}
		ops := []func() error{
			func() error {
				_, err := payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: id, Days: 2, At: now})
				return err
			},
			func() error {
				return s.ActivateDaysPromo(ctx, pr.ID, id, now, func(model.Promo) error {
					_, err := panel.UpdateUser(ctx, id, "u", panel.cur().Add(day), [16]byte{})
					return err
				})
			},
			func() error {
				_, _, err := s.CreatePayment(ctx, model.Payment{TelegramID: id, Username: "u", Month: 1, Price: 1}, func(model.Payment) error {
					time.Sleep(2 * time.Millisecond)
					return nil
				}, readPanel(panel, time.Millisecond))
				return err
			},
			func() error {
				_, err := s.ResolvePayment(ctx, pend(), model.PaymentConfirmed, now, func(model.Payment) ([]model.OutboxMessage, error) { return nil, nil })
				return err
			},
			func() error { return s.GrantAdvance(ctx, pend(), func(model.Payment) {}) },
		}
		var wg sync.WaitGroup
		for _, k := range rng.Perm(len(ops)) {
			delay := time.Duration(rng.IntN(15)) * time.Millisecond
			wg.Go(func() {
				time.Sleep(delay)
				// ожидаемые отказы (нет платежа, уже решён, промокод использован) не интересны — только зависание и дедлок
				if err := ops[k](); err != nil && (errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadlock")) {
					t.Errorf("iteration %d op %d: %v", i, k, err)
				}
			})
		}
		wg.Wait()
		cancel()
	}
}
