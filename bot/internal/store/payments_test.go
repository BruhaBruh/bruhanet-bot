package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
)

func newPay(id int64, prev time.Time) model.Payment {
	return model.Payment{TelegramID: id, Username: "u", Month: 1, Price: 150, PrevExpireAt: prev}
}

func noRollback(model.Payment) error { return nil }

func TestCreatePaymentCancelsPending(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	prev := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	first, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback)
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	var got model.Payment
	second, cancelled, err := s.CreatePayment(ctx, newPay(1, prev.Add(time.Hour)), func(old model.Payment) error {
		calls++
		got = old
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.ID != first.ID {
		t.Fatalf("rollback calls=%d old=%d want %d", calls, got.ID, first.ID)
	}
	if cancelled == nil || cancelled.ID != first.ID || cancelled.Status != model.PaymentCancelled {
		t.Fatalf("cancelled = %+v", cancelled)
	}
	if !second.PrevExpireAt.Equal(first.PrevExpireAt) {
		t.Fatalf("PrevExpireAt = %v want %v", second.PrevExpireAt, first.PrevExpireAt)
	}
	var n int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM payments WHERE status = 'pending'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending = %d err=%v", n, err)
	}
	var st model.PaymentStatus
	if err = s.pool.QueryRow(ctx, `SELECT status FROM payments WHERE id = $1`, first.ID).Scan(&st); err != nil || st != model.PaymentCancelled {
		t.Fatalf("first status = %q err=%v", st, err)
	}
	pend, err := s.PendingPayment(ctx, 1)
	if err != nil || pend.ID != second.ID {
		t.Fatalf("PendingPayment = %+v err=%v", pend, err)
	}
	if _, err = s.PendingPayment(ctx, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestCreatePaymentRollbackError(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	first, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if _, _, err = s.CreatePayment(ctx, newPay(1, time.Now()), func(model.Payment) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	pend, err := s.PendingPayment(ctx, 1)
	if err != nil || pend.ID != first.ID {
		t.Fatalf("pending = %+v err=%v", pend, err)
	}
	var n int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("payments = %d err=%v", n, err)
	}
}

func TestSubmitPaymentOnce(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SubmitPayment(ctx, p.ID, 77, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = s.SubmitPayment(ctx, p.ID, 78, time.Now()); !errors.Is(err, ErrNotPending) {
		t.Fatalf("want ErrNotPending, got %v", err)
	}
	got, err := s.PendingPayment(ctx, 1)
	if err != nil || got.AdminMessageID != 77 || got.SubmittedAt == nil {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if err = s.MarkAdvance(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestResolveConcurrent(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}

	var applied atomic.Int32
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			<-start
			_, errs[i] = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) error {
				applied.Add(1)
				return nil
			})
		})
	}
	close(start)
	wg.Wait()

	if applied.Load() != 1 {
		t.Fatalf("apply calls = %d", applied.Load())
	}
	var ok, notPending int
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, ErrNotPending):
			notPending++
		default:
			t.Fatalf("unexpected: %v", e)
		}
	}
	if ok != 1 || notPending != 1 {
		t.Fatalf("ok=%d notPending=%d", ok, notPending)
	}
	var st model.PaymentStatus
	var res *time.Time
	if err = s.pool.QueryRow(ctx, `SELECT status, resolved_at FROM payments WHERE id = $1`, p.ID).Scan(&st, &res); err != nil {
		t.Fatal(err)
	}
	if st != model.PaymentConfirmed || res == nil {
		t.Fatalf("status=%q resolved_at=%v", st, res)
	}
}

func TestResolveApplyError(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if _, err = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	got, err := s.PendingPayment(ctx, 1)
	if err != nil || got.ResolvedAt != nil {
		t.Fatalf("got %+v err=%v", got, err)
	}
	res, err := s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) error { return nil })
	if err != nil || res.Status != model.PaymentConfirmed {
		t.Fatalf("res %+v err=%v", res, err)
	}
	// повтор -> ErrNotPending с платежом
	res, err = s.ResolvePayment(ctx, p.ID, model.PaymentDeclined, time.Now(), func(model.Payment) error { return nil })
	if !errors.Is(err, ErrNotPending) || res.ID != p.ID {
		t.Fatalf("res %+v err=%v", res, err)
	}
	if _, err = s.ResolvePayment(ctx, p.ID, model.PaymentCancelled, time.Now(), nil); err == nil {
		t.Fatal("want error for invalid status")
	}
	if _, err = s.ResolvePayment(ctx, 9999, model.PaymentConfirmed, time.Now(), nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestLastBackupAt(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if _, err := s.LastBackupAt(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	late := time.Now().Truncate(time.Microsecond)
	for _, ts := range []time.Time{late.Add(-time.Hour), late} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO backups (created_at, size_bytes) VALUES ($1, 1)`, ts); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.LastBackupAt(ctx)
	if err != nil || !got.Equal(late) {
		t.Fatalf("got %v err=%v want %v", got, err, late)
	}
}

func TestUserStats(t *testing.T) {
	type pay struct {
		status model.PaymentStatus
		adv    bool
	}
	d, c, f := model.PaymentDeclined, model.PaymentCancelled, model.PaymentConfirmed
	tests := []struct {
		name        string
		pays        []pay
		excludeLast bool
		want        model.UserStats
	}{
		{"none", nil, false, model.UserStats{}},
		{"unpaid", []pay{{d, true}, {c, true}, {c, false}}, false, model.UserStats{UnpaidAdvances: 2, HasHistory: true}},
		{
			"after confirmed",
			[]pay{{d, true}, {f, false}, {d, true}},
			false,
			model.UserStats{UnpaidAdvances: 1, Confirmed: 1, ConfirmedSum: 150, HasHistory: true},
		},
		{"exclude only", []pay{{d, true}}, true, model.UserStats{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testStore(t)
			ctx := t.Context()
			var last int64
			for _, p := range tt.pays {
				if _, err := s.pool.Exec(ctx, `INSERT INTO users (telegram_id, username) VALUES (1, 'u') ON CONFLICT DO NOTHING`); err != nil {
					t.Fatal(err)
				}
				err := s.pool.QueryRow(ctx,
					`INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at, advance_granted, resolved_at)
					 VALUES (1, 'u', 1, 150, $1, now(), $2, now()) RETURNING id`, p.status, p.adv).Scan(&last)
				if err != nil {
					t.Fatal(err)
				}
			}
			var excl int64
			if tt.excludeLast {
				excl = last
			}
			got, err := s.UserStats(ctx, 1, excl)
			if err != nil {
				t.Fatal(err)
			}
			if (got.LastConfirmedAt != nil) != (tt.want.Confirmed > 0) || (got.FirstConfirmedAt != nil) != (tt.want.Confirmed > 0) {
				t.Fatalf("confirmed times: %+v", got)
			}
			got.LastConfirmedAt, got.FirstConfirmedAt = nil, nil
			if got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}
