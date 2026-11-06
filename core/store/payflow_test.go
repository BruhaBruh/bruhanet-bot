package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

// racePanel пишет в панель с задержкой: транзакция открыта, пока второй участник ждёт блокировку строки.
type racePanel struct {
	mu      sync.Mutex
	expires []time.Time
	limits  []int
	err     error
}

func (p *racePanel) SetDeviceLimit(_ context.Context, _ int64, limit int) (*api.UserItemInfo, error) {
	time.Sleep(100 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	p.limits = append(p.limits, limit)
	return &api.UserItemInfo{}, nil
}

func (p *racePanel) UpdateUser(_ context.Context, _ int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	time.Sleep(100 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return nil, p.err
	}
	p.expires = append(p.expires, expireAt)
	return &api.UserItemInfo{}, nil
}

func (p *racePanel) writes() []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time{}, p.expires...)
}

func outboxTypes(t *testing.T, s *Store) []string {
	t.Helper()
	items, err := s.OutboxBatch(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.EventType
	}
	return out
}

// Бот и панель решают один платёж одновременно: один исход, второй получает ErrNotPending.
func TestPaymentRaceBotAndPanel(t *testing.T) {
	for _, tc := range []struct {
		name               string
		botApprove, panelA bool
	}{
		{"both confirm", true, true},
		{"bot confirms, panel declines", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testStore(t)
			ctx := t.Context()
			prev := time.Now().Add(48 * time.Hour).Truncate(time.Microsecond)
			p, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.SubmitPayment(ctx, p.ID, 77, "file", time.Now()); err != nil {
				t.Fatal(err)
			}
			panel := &racePanel{}
			at := time.Now()
			errs := make([]error, 2)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i, d := range []payflow.Decision{
				{ID: p.ID, Approve: tc.botApprove, At: at},
				{ID: p.ID, Approve: tc.panelA, At: at, AdminID: 99},
			} {
				wg.Go(func() {
					<-start
					_, errs[i] = payflow.Resolve(ctx, s, panel, d)
				})
			}
			close(start)
			wg.Wait()

			won := 0
			for _, err := range errs {
				switch {
				case err == nil:
					won++
				case !errors.Is(err, ErrNotPending):
					t.Fatalf("errs = %v", errs)
				}
			}
			writes := panel.writes()
			if won != 1 || len(writes) != 1 {
				t.Fatalf("errs = %v, panel writes = %v", errs, writes)
			}
			got, err := s.PaymentByID(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			// срок в панели и статус — от одного исхода
			wantExpire := prev
			if got.Status == model.PaymentConfirmed {
				wantExpire = payflow.ExtendedExpire(prev, at, got.Month)
			}
			if !writes[0].Equal(wantExpire) {
				t.Fatalf("status %s: expire %v, want %v", got.Status, writes[0], wantExpire)
			}
			types := outboxTypes(t, s)
			if errs[1] == nil { // победила панель
				if len(types) != 2 || types[0] != payflow.EventResult || types[1] != payflow.EventAdminClosed {
					t.Fatalf("outbox = %v", types)
				}
			} else if len(types) != 1 || types[0] != payflow.EventResult {
				t.Fatalf("outbox = %v", types)
			}
		})
	}
}

// Повтор и ошибка панели не трогают дату; после ошибки панели повтор считает ту же дату.
func TestPaymentRepeatKeepsDates(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	prev := time.Now().Add(48 * time.Hour).Truncate(time.Microsecond)
	p, _, err := s.CreatePayment(ctx, newPay(1, prev), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().Truncate(time.Microsecond)
	d := payflow.Decision{ID: p.ID, Approve: true, At: at, AdminID: 99}

	panel := &racePanel{err: errors.New("panel is down")}
	if _, err = payflow.Resolve(ctx, s, panel, d); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if got, _ := s.PaymentByID(ctx, p.ID); got.Status != model.PaymentPending || got.ResolvedAt != nil {
		t.Fatalf("panel error must roll back: %+v", got)
	}
	if types := outboxTypes(t, s); len(types) != 0 {
		t.Fatalf("outbox after rollback = %v", types)
	}

	panel = &racePanel{}
	first, err := payflow.Resolve(ctx, s, panel, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID, Approve: true, At: at.Add(time.Hour), AdminID: 99}); !errors.Is(err, ErrNotPending) {
		t.Fatalf("repeat: %v", err)
	}
	writes := panel.writes()
	if len(writes) != 1 || !writes[0].Equal(payflow.ExtendedExpire(prev, at, 1)) {
		t.Fatalf("writes = %v", writes)
	}
	again, err := s.PaymentByID(ctx, p.ID)
	if err != nil || !again.ResolvedAt.Equal(*first.ResolvedAt) {
		t.Fatalf("resolved_at changed: %v -> %v (%v)", first.ResolvedAt, again.ResolvedAt, err)
	}
	if types := outboxTypes(t, s); len(types) != 2 {
		t.Fatalf("outbox = %v", types)
	}
	if _, err = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID + 100, At: at}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

// Запись аванса и решение из панели сериализуются: решение идёт после аванса и его не затирает.
func TestGrantAdvanceHoldsRow(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	var mu sync.Mutex
	mark := func(step string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, step)
	}
	var started atomic.Bool
	done := make(chan error, 1)
	err = s.GrantAdvance(ctx, p.ID, func(locked model.Payment) {
		if !locked.AdvanceGranted {
			t.Error("флаг ставится до записи в панель")
		}
		started.Store(true)
		go func() {
			_, err := s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) ([]model.OutboxMessage, error) {
				mark("resolve")
				return okApply(model.Payment{})
			})
			done <- err
		}()
		time.Sleep(200 * time.Millisecond) // решение успевает упереться в FOR UPDATE
		mark("advance")
	})
	if err != nil || !started.Load() {
		t.Fatalf("GrantAdvance: %v", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "advance" || order[1] != "resolve" {
		t.Fatalf("order = %v", order)
	}

	// платёж уже решён: панель не трогаем
	err = s.GrantAdvance(ctx, p.ID, func(model.Payment) { t.Error("apply на решённом платеже") })
	if !errors.Is(err, ErrNotPending) {
		t.Fatalf("resolved: %v", err)
	}
	if err = s.GrantAdvance(ctx, p.ID+100, func(model.Payment) {}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

// Миграция убирает id из username: только совпадающий со своим id.
func TestUsernameMigrationClearsIDs(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if _, _, err := s.CreatePayment(ctx, model.Payment{TelegramID: 5, Username: "5", Month: 1, PrevExpireAt: time.Now()}, noRollback, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreatePayment(ctx, model.Payment{TelegramID: 6, Username: "ann", Month: 1, PrevExpireAt: time.Now()}, noRollback, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.ReadFile("migrations/00010_payment_usernames.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")
	if _, err = s.pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]string{5: "", 6: "ann"} {
		p, err := s.PendingPayment(ctx, id)
		if err != nil || p.Username != want {
			t.Fatalf("%d: %q %v", id, p.Username, err)
		}
	}
}
