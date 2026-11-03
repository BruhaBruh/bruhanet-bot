package store

import (
	"errors"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

func addUser(t *testing.T, s *Store, id int64, name string) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO users (telegram_id, username) VALUES ($1, $2)`, id, name); err != nil {
		t.Fatal(err)
	}
}

func addPayment(t *testing.T, s *Store, id int64, status model.PaymentStatus, price int, created time.Time, resolved *time.Time) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(),
		`INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at, created_at, resolved_at)
		 VALUES ($1, 'u', 1, $2, $3, $4, $4, $5)`, id, price, status, created, resolved); err != nil {
		t.Fatal(err)
	}
}

func usernames(us []model.UserSummary) []string {
	r := make([]string, 0, len(us))
	for _, u := range us {
		r = append(r, u.Username)
	}
	return r
}

func TestSearchUsersLiteral(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a_b")
	addUser(t, s, 2, "axb")
	got, err := s.SearchUsers(t.Context(), "a_b", 50)
	if err != nil || len(got) != 1 || got[0].Username != "a_b" {
		t.Fatalf("a_b: %v %v", usernames(got), err)
	}
	got, err = s.SearchUsers(t.Context(), "%", 50)
	if err != nil || len(got) != 0 {
		t.Fatalf("%%: %v %v", usernames(got), err)
	}
	got, err = s.SearchUsers(t.Context(), `\`, 50)
	if err != nil || len(got) != 0 {
		t.Fatalf(`\: %v %v`, usernames(got), err)
	}
	got, err = s.SearchUsers(t.Context(), "@AXB", 50)
	if err != nil || len(got) != 1 || got[0].Username != "axb" {
		t.Fatalf("@AXB: %v %v", usernames(got), err)
	}
}

func TestSearchUsersByID(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 777, "someone")
	addUser(t, s, 5, "x777y")
	addUser(t, s, 6, "other")
	got, err := s.SearchUsers(t.Context(), "777", 50)
	if err != nil || len(got) != 2 {
		t.Fatalf("777: %v %v", usernames(got), err)
	}
	got, err = s.SearchUsers(t.Context(), "", 50)
	if err != nil || len(got) != 3 {
		t.Fatalf("empty: %v %v", usernames(got), err)
	}
}

func TestSearchUsersStatsAndByID(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a")
	now := time.Now()
	addPayment(t, s, 1, model.PaymentConfirmed, 150, now, &now)
	addPayment(t, s, 1, model.PaymentConfirmed, 300, now, &now)
	addPayment(t, s, 1, model.PaymentDeclined, 999, now, &now)
	u, err := s.UserByID(t.Context(), 1)
	if err != nil || u.Confirmed != 2 || u.ConfirmedSum != 450 {
		t.Fatalf("%+v %v", u, err)
	}
	if _, err = s.UserByID(t.Context(), 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	ps, err := s.PaymentsByUser(t.Context(), 1)
	if err != nil || len(ps) != 3 || ps[0].ID < ps[2].ID {
		t.Fatalf("%+v %v", ps, err)
	}
}

func TestListPaymentsFilter(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a")
	d := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC) }
	addPayment(t, s, 1, model.PaymentConfirmed, 100, d(1), nil)
	addPayment(t, s, 1, model.PaymentConfirmed, 100, d(5), nil)
	addPayment(t, s, 1, model.PaymentDeclined, 100, d(5), nil)
	addPayment(t, s, 1, model.PaymentConfirmed, 100, d(10), nil)

	ps, total, err := s.ListPayments(t.Context(), model.PaymentFilter{Status: model.PaymentConfirmed, From: d(2), To: d(10), Limit: 50})
	if err != nil || total != 1 || len(ps) != 1 || !ps[0].CreatedAt.Equal(d(5)) {
		t.Fatalf("status+period: %+v total=%d %v", ps, total, err)
	}
	ps, total, err = s.ListPayments(t.Context(), model.PaymentFilter{Limit: 2, Offset: 2})
	if err != nil || total != 4 || len(ps) != 2 {
		t.Fatalf("paging: %d total=%d %v", len(ps), total, err)
	}
	if ps[0].ID < ps[1].ID {
		t.Fatal("want ORDER BY id DESC")
	}
}

func TestRevenue(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a")
	d := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC) }
	r := func(day int) *time.Time { x := d(day); return &x }
	addPayment(t, s, 1, model.PaymentConfirmed, 100, d(1), r(5))
	addPayment(t, s, 1, model.PaymentConfirmed, 200, d(1), r(10)) // to исключён
	addPayment(t, s, 1, model.PaymentConfirmed, 400, d(1), r(1))  // до from
	addPayment(t, s, 1, model.PaymentDeclined, 800, d(1), r(6))
	got, err := s.Revenue(t.Context(), d(2), d(10))
	if err != nil || got != 100 {
		t.Fatalf("revenue = %d %v", got, err)
	}
}

func TestUserByUsername(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "Alice")
	addUser(t, s, 2, "alice_x")
	for _, q := range []string{"alice", "@ALICE", " @Alice "} {
		u, err := s.UserByUsername(t.Context(), q)
		if err != nil || u.TelegramID != 1 {
			t.Fatalf("%q: %+v %v", q, u, err)
		}
	}
	for _, q := range []string{"ali", "", "@"} {
		if _, err := s.UserByUsername(t.Context(), q); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%q: want ErrNotFound, got %v", q, err)
		}
	}
}
