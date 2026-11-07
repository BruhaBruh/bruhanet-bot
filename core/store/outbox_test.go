package store

import (
	"errors"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/model"
)

// Внутри одного вида — по порядку id.
func TestOutboxFIFO(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	kb := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "ok", CallbackData: "x"}}}}
	err := s.EnqueueMany(ctx, []model.OutboxMessage{
		{EventType: "reminder", ChatID: 1, Text: "a", ParseMode: "MarkdownV2", Keyboard: kb},
		{EventType: "broadcast", ChatID: 2, FromChatID: 9, MessageID: 5, Broadcast: "u"},
		{EventType: "broadcast", ChatID: 3, Text: "c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v err=%v", got, err)
	}
	if got[0].ID >= got[1].ID || got[0].ChatID != 1 || got[1].ChatID != 2 {
		t.Fatalf("order: %+v", got)
	}
	m := got[0]
	if m.EventType != "reminder" || m.Text != "a" || m.ParseMode != "MarkdownV2" ||
		m.Keyboard == nil || m.Keyboard.InlineKeyboard[0][0].CallbackData != "x" {
		t.Fatalf("round-trip: %+v", m)
	}
	if c := got[1]; c.FromChatID != 9 || c.MessageID != 5 || c.Broadcast != "u" || c.EventType != "broadcast" {
		t.Fatalf("round-trip: %+v", c)
	}
	if n, err := s.OutboxCount(ctx); err != nil || n != 3 {
		t.Fatalf("count %d err=%v", n, err)
	}
}

func TestOutboxDelete(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if err := s.Enqueue(ctx, model.OutboxMessage{EventType: "reminder", ChatID: 1, Text: "a"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v err=%v", got, err)
	}
	if err = s.DeleteOutbox(ctx, got[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteOutbox(ctx, got[0].ID); err != nil { // идемпотентно
		t.Fatal(err)
	}
	if n, err := s.OutboxCount(ctx); err != nil || n != 0 {
		t.Fatalf("count %d err=%v", n, err)
	}
}

func TestResolveEnqueues(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	_, err = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(),
		func(model.Payment) ([]model.OutboxMessage, error) { return nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n, _ := s.OutboxCount(ctx); n != 0 {
		t.Fatalf("outbox = %d after failed apply", n)
	}
	if _, err = s.PendingPayment(ctx, 1); err != nil {
		t.Fatalf("payment must stay pending: %v", err)
	}

	_, err = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(),
		func(model.Payment) ([]model.OutboxMessage, error) {
			return []model.OutboxMessage{{EventType: "payment_result", ChatID: 1, Text: "ok"}}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 1 || got[0].EventType != "payment_result" || got[0].ChatID != 1 {
		t.Fatalf("got %+v err=%v", got, err)
	}
}

func TestEnqueueRejectsEmpty(t *testing.T) {
	s := &Store{} // до пула дело не доходит
	for _, m := range []model.OutboxMessage{{}, {EventType: "x"}, {ChatID: 1}} {
		if err := s.Enqueue(t.Context(), m); !errors.Is(err, ErrEmptyMessage) {
			t.Fatalf("Enqueue(%+v) = %v", m, err)
		}
	}
	if err := s.EnqueueMany(t.Context(), []model.OutboxMessage{{EventType: "x", ChatID: 1}, {}}); !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("EnqueueMany = %v", err)
	}
}

// Платёж и другие уведомления не ждут хвост рассылки: строки рассылки идут после всего остального, даже если
// положены раньше.
func TestOutboxPriorityOverBroadcast(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	err := s.EnqueueMany(ctx, []model.OutboxMessage{
		{EventType: "broadcast", ChatID: 1, Text: "b1"},
		{EventType: "broadcast", ChatID: 2, Text: "b2"},
		{EventType: "payment_result", ChatID: 3, TextKey: "payment.confirmed"},
		{EventType: "broadcast", ChatID: 4, Text: "b3"},
		{EventType: "direct", ChatID: 5, Text: "d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 5 {
		t.Fatalf("got %v err=%v", got, err)
	}
	order := make([]int64, 0, len(got))
	for _, it := range got {
		order = append(order, it.ChatID)
	}
	if want := []int64{3, 5, 1, 2, 4}; !slicesEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	// пачка в 2 строки: сначала два небольших уведомления, рассылка ждёт
	got, _ = s.OutboxBatch(ctx, 2)
	if len(got) != 2 || got[0].ChatID != 3 || got[1].ChatID != 5 {
		t.Fatalf("small batch: %+v", got)
	}
}

func slicesEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
