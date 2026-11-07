package store

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/panelsync"
)

func TestUserContact(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if c, err := s.UserContact(ctx, 1); err != nil || c != (model.UserContact{}) {
		t.Fatalf("no user: %+v %v", c, err)
	}
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 1, Username: "ann", FirstName: "Анна", At: at}); err != nil {
		t.Fatal(err)
	}
	c, err := s.UserContact(ctx, 1)
	if err != nil || c.FirstName != "Анна" || c.Username != "ann" || c.ExpireAt != nil {
		t.Fatalf("%+v %v", c, err)
	}
	if err := s.SetUserPanel(ctx, 1, at, "ACTIVE", at); err != nil {
		t.Fatal(err)
	}
	if c, err = s.UserContact(ctx, 1); err != nil || c.ExpireAt == nil || !c.ExpireAt.Equal(at) {
		t.Fatalf("%+v %v", c, err)
	}
}

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

func TestDeliveryMarks(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	addUser(t, s, 1, "a")
	for _, f := range []func(context.Context, int64) error{s.MarkBotBlocked, s.AddBroadcastReceived, s.AddBroadcastReceived} {
		if err := f(ctx, 1); err != nil {
			t.Fatal(err)
		}
		if err := f(ctx, 999); err != nil { // юзера нет — не ошибка
			t.Fatal(err)
		}
	}
	u, err := s.UserByID(ctx, 1)
	if err != nil || u.BlockedBotAt == nil || u.BroadcastsReceived != 2 {
		t.Fatalf("%+v %v", u, err)
	}
	first := *u.BlockedBotAt
	if err = s.MarkBotBlocked(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if u, err = s.UserByID(ctx, 1); err != nil || !u.BlockedBotAt.Equal(first) {
		t.Fatalf("first time lost: %+v %v", u, err)
	}
	if err = s.TouchUser(ctx, model.UserSeen{TelegramID: 1, Username: "a", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if u, err = s.UserByID(ctx, 1); err != nil || u.BlockedBotAt != nil {
		t.Fatalf("not cleared: %+v %v", u, err)
	}
}

type syncPanel struct {
	users []api.UserItemInfo
	err   error
}

func (p syncPanel) Users(context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		for _, u := range p.users {
			if !yield(u, nil) {
				return
			}
		}
		if p.err != nil {
			yield(api.UserItemInfo{}, p.err)
		}
	}
}

// Review Focus: при недоступной панели данные не затираются, даже если часть юзеров уже прочитана.
func TestPanelSyncRunKeepsDataOnPanelFailure(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	at := time.Now()
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	panelUser := func(id int, status api.UserItemInfoStatus, e time.Time) api.UserItemInfo {
		return api.UserItemInfo{Username: "u", Status: status, ExpireAt: e, CreatedAt: exp, TelegramId: api.NewNilInt(id)}
	}
	res, err := panelsync.Run(ctx, syncPanel{users: []api.UserItemInfo{panelUser(1, api.UserItemInfoStatusACTIVE, exp), panelUser(2, api.UserItemInfoStatusACTIVE, exp)}}, s, time.Now)
	if err != nil || res.Total != 2 || res.Updated != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	before, err := s.LastPanelSync(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Падает на втором юзере, первый уже прочитан с другим сроком.
	_, err = panelsync.Run(ctx, syncPanel{users: []api.UserItemInfo{panelUser(1, api.UserItemInfoStatusDISABLED, at)}, err: errors.New("boom")}, s, time.Now)
	if !errors.Is(err, panelsync.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if r := loadPanelRow(t, s, 1); *r.status != "ACTIVE" || !r.expire.Equal(exp) {
		t.Fatalf("overwritten: %+v", r)
	}
	// Пустой ответ панели не снимает статусы.
	if _, err = panelsync.Run(ctx, syncPanel{}, s, time.Now); !errors.Is(err, panelsync.ErrPanel) {
		t.Fatalf("empty: %v", err)
	}
	if r := loadPanelRow(t, s, 2); r.status == nil {
		t.Fatalf("wiped: %+v", r)
	}
	after, err := s.LastPanelSync(ctx)
	if err != nil || !after.FinishedAt.Equal(before.FinishedAt) {
		t.Fatalf("last sync moved: %+v %v", after, err)
	}

	// Второй запуск во время первого — отдельная ошибка, не очередь.
	err = s.WithPanelSyncLock(ctx, func(ctx context.Context) error {
		_, err := panelsync.Run(ctx, syncPanel{err: errors.New("must not read")}, s, time.Now)
		return err
	})
	if !errors.Is(err, ErrSyncRunning) {
		t.Fatalf("overlap: %v", err)
	}
}

func TestNamesDueAndSave(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	week := at.Add(-7 * 24 * time.Hour)
	// 1: без имени, не писал; 2: без имени, писал недавно; 3: с именем; 4: без имени, спрашивали вчера
	for id, u := range map[int64]model.UserSeen{
		2: {TelegramID: 2, At: at}, 3: {TelegramID: 3, FirstName: "Ян", At: at}, 4: {TelegramID: 4, Username: "dd", At: at},
	} {
		if err := s.TouchUser(ctx, u); err != nil {
			t.Fatal(id, err)
		}
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO users (telegram_id, username) VALUES (1, 'old')`); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveChatName(ctx, 4, "", "", "", at.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	ids, err := s.NamesDue(ctx, week, 10)
	if err != nil || len(ids) != 2 || ids[0] != 2 || ids[1] != 1 {
		t.Fatalf("due: %v %v", ids, err)
	}
	if ids, _ = s.NamesDue(ctx, week, 1); len(ids) != 1 {
		t.Fatalf("limit: %v", ids)
	}
	// спрашивали давно — снова в выборке
	if ids, _ = s.NamesDue(ctx, at, 10); len(ids) != 3 {
		t.Fatalf("stale: %v", ids)
	}

	// имя записано и попытка отмечена; username без имени не стирается пустым
	if err = s.SaveChatName(ctx, 1, "", "Ольга", "С", at); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveChatName(ctx, 1, "", "", "", at); err != nil {
		t.Fatal(err)
	}
	var first, last, user string
	if err = s.pool.QueryRow(ctx, `SELECT first_name, last_name, username FROM users WHERE telegram_id = 1`).Scan(&first, &last, &user); err != nil {
		t.Fatal(err)
	}
	if first != "Ольга" || last != "С" || user != "old" {
		t.Fatalf("saved: %q %q %q", first, last, user)
	}
	// новый непустой username заменяет прежний
	if err = s.SaveChatName(ctx, 1, "new", "", "", at); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT username FROM users WHERE telegram_id = 1`).Scan(&user); err != nil || user != "new" {
		t.Fatalf("username: %q %v", user, err)
	}
	if ids, _ = s.NamesDue(ctx, at.Add(time.Hour), 10); len(ids) != 2 {
		t.Fatalf("after save: %v", ids) // 1 теперь с именем и выпал
	}
	// панельная синхронизация не стирает имя и username
	if _, err = s.ApplyPanelSync(ctx, []model.PanelUser{{TelegramID: 1, Username: "", Status: "ACTIVE", ExpireAt: at, CreatedAt: at}}, at.Add(time.Hour), at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `SELECT first_name, username FROM users WHERE telegram_id = 1`).Scan(&first, &user); err != nil || first != "Ольга" || user != "new" {
		t.Fatalf("sync wiped: %q %q %v", first, user, err)
	}
}
