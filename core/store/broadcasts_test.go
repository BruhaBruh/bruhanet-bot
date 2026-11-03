package store

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

// Review Focus 1: повтор с тем же ключом не создаёт ни рассылки, ни строк outbox.
func TestCreateBroadcastIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	d := model.BroadcastDraft{IdempotencyKey: uuid.New(), Text: "<b>hi</b>", Photo: "file1"}
	b, created, err := s.CreateBroadcast(ctx, d, []int64{10, 20}, 1)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if b.Total != 2 || b.Text != d.Text || b.Photo != "file1" || b.FinishedAt != nil || b.CreatedAt.IsZero() {
		t.Fatalf("broadcast: %+v", b)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("outbox %+v err=%v", got, err)
	}
	for i, chat := range []int64{10, 20} {
		m := got[i]
		if m.EventType != "broadcast" || m.ChatID != chat || m.Text != d.Text || m.Photo != "file1" ||
			m.ParseMode != "HTML" || m.Broadcast != b.ID.String() {
			t.Fatalf("row %d: %+v", i, m)
		}
	}
	if m := got[2]; m.EventType != "broadcast_summary" || m.ChatID != 1 || m.Broadcast != b.ID.String() || m.ParseMode != "HTML" {
		t.Fatalf("summary: %+v", m)
	}

	again, created, err := s.CreateBroadcast(ctx, model.BroadcastDraft{IdempotencyKey: d.IdempotencyKey, Text: "other"}, []int64{30}, 1)
	if err != nil || created || again.ID != b.ID || again.Text != d.Text || again.Total != 2 {
		t.Fatalf("again=%+v created=%v err=%v", again, created, err)
	}
	if n, _ := s.OutboxCount(ctx); n != 3 {
		t.Fatalf("outbox = %d after repeat", n)
	}
}

// Review Focus 2: счётчики в БД, итог из них.
func TestBroadcastCounters(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	b, _, err := s.CreateBroadcast(ctx, model.BroadcastDraft{IdempotencyKey: uuid.New(), Text: "x"}, []int64{1, 2, 3}, 9)
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []bool{true, true, false} {
		if err = s.BroadcastDelivered(ctx, b.ID, ok); err != nil {
			t.Fatal(err)
		}
	}
	f, err := s.FinishBroadcast(ctx, b.ID)
	if err != nil || f.Sent != 2 || f.Failed != 1 || f.Total != 3 || f.FinishedAt == nil || f.Photo != "" {
		t.Fatalf("finish %+v err=%v", f, err)
	}
	if err = s.BroadcastDelivered(ctx, uuid.New(), true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown delivered: %v", err)
	}
	if _, err = s.FinishBroadcast(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown finish: %v", err)
	}
}

func TestListBroadcasts(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	ids := make([]uuid.UUID, 0, 3)
	for range 3 {
		b, _, err := s.CreateBroadcast(ctx, model.BroadcastDraft{IdempotencyKey: uuid.New(), Text: "x"}, []int64{1}, 9)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	got, total, err := s.ListBroadcasts(ctx, 2, 0)
	if err != nil || total != 3 || len(got) != 2 || got[0].ID != ids[2] || got[1].ID != ids[1] {
		t.Fatalf("page1 %+v total=%d err=%v", got, total, err)
	}
	got, total, err = s.ListBroadcasts(ctx, 2, 2)
	if err != nil || total != 3 || len(got) != 1 || got[0].ID != ids[0] {
		t.Fatalf("page2 %+v total=%d err=%v", got, total, err)
	}
}

func TestDirectMessages(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	id1, err := s.CreateDirect(ctx, 5, "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateDirect(ctx, 5, "", "pic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDirect(ctx, 6, "other", ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("outbox %+v err=%v", got, err)
	}
	if m := got[1]; m.EventType != "direct" || m.ChatID != 5 || m.Photo != "pic" || m.DirectID != id2 || m.ParseMode != "HTML" {
		t.Fatalf("direct row: %+v", m)
	}

	if err = s.DirectDelivered(ctx, id1, true); err != nil {
		t.Fatal(err)
	}
	if err = s.DirectDelivered(ctx, id2, false); err != nil {
		t.Fatal(err)
	}
	if err = s.DirectDelivered(ctx, 999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	ms, err := s.DirectMessages(ctx, 5)
	if err != nil || len(ms) != 2 {
		t.Fatalf("messages %+v err=%v", ms, err)
	}
	if ms[0].ID != id2 || ms[0].Status != model.DirectFailed || ms[0].Photo != "pic" ||
		ms[1].ID != id1 || ms[1].Status != model.DirectSent || ms[1].Text != "hello" || ms[1].TelegramID != 5 || ms[1].CreatedAt.IsZero() {
		t.Fatalf("messages: %+v", ms)
	}
	if ms, err = s.DirectMessages(ctx, 7); err != nil || len(ms) != 0 {
		t.Fatalf("empty: %+v err=%v", ms, err)
	}
}
