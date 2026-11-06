package store

import (
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

func TestTouchUser(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

	if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, Username: "ann", FirstName: "Анна", LastName: "К", At: at}); err != nil {
		t.Fatal(err)
	}
	var first, last *string
	var seen, blocked *time.Time
	load := func() {
		t.Helper()
		if err := s.pool.QueryRow(ctx, `SELECT first_name, last_name, last_seen_at, blocked_bot_at FROM users WHERE telegram_id = 1`).
			Scan(&first, &last, &seen, &blocked); err != nil {
			t.Fatal(err)
		}
	}
	load()
	if first == nil || *first != "Анна" || last == nil || *last != "К" || seen == nil || !seen.Equal(at) {
		t.Fatalf("insert: %v %v %v", first, last, seen)
	}

	// Написал снова: имя новое, last_name убран, блокировка снята, created_at на месте.
	if _, err := s.pool.Exec(ctx, `UPDATE users SET blocked_bot_at = now(), created_at = '2020-01-01' WHERE telegram_id = 1`); err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, Username: "ann2", FirstName: "Аня", At: later}); err != nil {
		t.Fatal(err)
	}
	load()
	if *first != "Аня" || last != nil || !seen.Equal(later) || blocked != nil {
		t.Fatalf("update: %v %v %v %v", *first, last, seen, blocked)
	}
	var created time.Time
	if err := s.pool.QueryRow(ctx, `SELECT created_at FROM users WHERE telegram_id = 1`).Scan(&created); err != nil || created.Year() != 2020 {
		t.Fatalf("created_at: %v %v", created, err)
	}
}
