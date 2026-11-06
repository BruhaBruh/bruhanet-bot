package routes

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func TestWriteThrough(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	rw, pay := tgtest.NewRemnawave(func() time.Time { return now }), tgtest.NewPayments()
	w := writeThrough{rw, pay}

	if _, err := w.CreateUser(t.Context(), 7, "ann", uuid.Nil); err != nil {
		t.Fatal(err)
	}
	exp := now.Add(24 * time.Hour)
	if _, err := w.UpdateUser(t.Context(), 7, "ann", exp, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	got := pay.PanelWrites()
	if len(got) != 2 || !got[0].ExpireAt.Equal(now) || !got[1].ExpireAt.Equal(exp) || got[1].Status != "ACTIVE" || got[1].TelegramID != 7 {
		t.Fatalf("writes = %+v", got)
	}

	// Панель отказала — писать нечего, ошибка доходит до хендлера.
	boom := errors.New("boom")
	rw.FailUpdateWith(boom)
	if _, err := w.UpdateUser(t.Context(), 7, "ann", exp, uuid.Nil); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(pay.PanelWrites()); n != 2 {
		t.Fatalf("writes after failure = %d", n)
	}
}
