package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

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

type panelRow struct {
	username string
	status   *string
	expire   *time.Time
	synced   *time.Time
	created  time.Time
}

func loadPanelRow(t *testing.T, s *Store, id int64) panelRow {
	t.Helper()
	var r panelRow
	if err := s.pool.QueryRow(t.Context(), `SELECT username, panel_status, expire_at, synced_at, created_at FROM users WHERE telegram_id = $1`, id).
		Scan(&r.username, &r.status, &r.expire, &r.synced, &r.created); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestApplyPanelSync(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	finish := start.Add(time.Second)
	exp := start.Add(72 * time.Hour)
	reg := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	addUser(t, s, 1, "existing") // обновится, имя останется
	addUser(t, s, 3, "gone")     // был в панели, больше нет
	addUser(t, s, 4, "fresh")    // запись после started свежее снимка
	addUser(t, s, 5, "visitor")  // не был в панели: не трогаем
	if err := s.SetUserPanel(ctx, 3, exp, "ACTIVE", start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPanel(ctx, 4, exp, "DISABLED", start.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	n, err := s.ApplyPanelSync(ctx, []model.PanelUser{
		{TelegramID: 1, Username: "panelname", Status: "ACTIVE", ExpireAt: exp, CreatedAt: reg},
		{TelegramID: 2, Username: "new", Status: "EXPIRED", ExpireAt: exp, CreatedAt: reg},
		{TelegramID: 4, Username: "fresh", Status: "ACTIVE", ExpireAt: start, CreatedAt: reg},
	}, start, finish)
	if err != nil || n != 3 { // 1, 2 и снятие с 3
		t.Fatalf("n=%d err=%v", n, err)
	}

	r := loadPanelRow(t, s, 1)
	if r.username != "existing" || *r.status != "ACTIVE" || !r.expire.Equal(exp) || !r.synced.Equal(finish) {
		t.Fatalf("existing: %+v", r)
	}
	if r := loadPanelRow(t, s, 2); r.username != "new" || *r.status != "EXPIRED" || !r.created.Equal(reg) {
		t.Fatalf("inserted: %+v", r)
	}
	if r := loadPanelRow(t, s, 3); r.status != nil || r.expire != nil {
		t.Fatalf("gone: %+v", r)
	}
	if r := loadPanelRow(t, s, 4); *r.status != "DISABLED" || !r.expire.Equal(exp) {
		t.Fatalf("fresh overwritten: %+v", r)
	}
	if r := loadPanelRow(t, s, 5); r.status != nil || r.synced != nil {
		t.Fatalf("visitor: %+v", r)
	}

	last, err := s.LastPanelSync(ctx)
	if err != nil || last.Total != 3 || last.Updated != 3 || !last.FinishedAt.Equal(finish) {
		t.Fatalf("last: %+v %v", last, err)
	}
}

func TestLastPanelSyncEmpty(t *testing.T) {
	if _, err := testStore(t).LastPanelSync(t.Context()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// Вызов панели идёт внутри транзакции, держащей строку юзера (CreatePayment): запись не должна её ждать.
func TestSetUserPanelSkipsLockedRow(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a")
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	err := s.tx(t.Context(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE users SET username = 'b' WHERE telegram_id = 1`); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		return s.SetUserPanel(ctx, 1, exp, "ACTIVE", exp)
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := loadPanelRow(t, s, 1); r.status != nil {
		t.Fatalf("locked row written: %+v", r)
	}
	if err = s.SetUserPanel(t.Context(), 1, exp, "ACTIVE", exp); err != nil {
		t.Fatal(err)
	}
	if r := loadPanelRow(t, s, 1); *r.status != "ACTIVE" || !r.expire.Equal(exp) {
		t.Fatalf("free row: %+v", r)
	}
}

func TestWithPanelSyncLock(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	err := s.WithPanelSyncLock(ctx, func(ctx context.Context) error {
		if err := s.WithPanelSyncLock(ctx, func(context.Context) error {
			t.Error("second holder ran")
			return nil
		}); !errors.Is(err, ErrSyncRunning) {
			t.Errorf("nested: %v", err)
		}
		return errors.New("boom") // ошибка тоже снимает блокировку
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v", err)
	}
	ran := false
	if err = s.WithPanelSyncLock(ctx, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("after release: ran=%v err=%v", ran, err)
	}
}
