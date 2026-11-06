package store

import (
	"errors"
	"slices"
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

func searchUsers(t *testing.T, s *Store, q string) ([]model.UserSummary, error) {
	t.Helper()
	us, _, err := s.ListUsers(t.Context(), model.UserFilter{Q: q, Now: time.Now(), Limit: 50})
	return us, err
}

func TestSearchUsersLiteral(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a_b")
	addUser(t, s, 2, "axb")
	got, err := searchUsers(t, s, "a_b")
	if err != nil || len(got) != 1 || got[0].Username != "a_b" {
		t.Fatalf("a_b: %v %v", usernames(got), err)
	}
	got, err = searchUsers(t, s, "%")
	if err != nil || len(got) != 0 {
		t.Fatalf("%%: %v %v", usernames(got), err)
	}
	got, err = searchUsers(t, s, `\`)
	if err != nil || len(got) != 0 {
		t.Fatalf(`\: %v %v`, usernames(got), err)
	}
	got, err = searchUsers(t, s, "@AXB")
	if err != nil || len(got) != 1 || got[0].Username != "axb" {
		t.Fatalf("@AXB: %v %v", usernames(got), err)
	}
}

func TestSearchUsersByID(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 777, "someone")
	addUser(t, s, 5, "x777y")
	addUser(t, s, 6, "other")
	got, err := searchUsers(t, s, "777")
	if err != nil || len(got) != 2 {
		t.Fatalf("777: %v %v", usernames(got), err)
	}
	got, err = searchUsers(t, s, "")
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

func listIDs(t *testing.T, s *Store, f model.UserFilter) ([]int64, int) {
	t.Helper()
	if f.Limit == 0 {
		f.Limit = 50
	}
	us, total, err := s.ListUsers(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(us))
	for _, u := range us {
		ids = append(ids, u.TelegramID)
	}
	return ids, total
}

func TestListUsersPagination(t *testing.T) {
	s := testStore(t)
	for i := int64(1); i <= 5; i++ {
		addUser(t, s, i, "u")
	}
	// created_at одинаковый: порядок задаёт telegram_id, страницы не пересекаются.
	ids, total := listIDs(t, s, model.UserFilter{Limit: 2, Offset: 2})
	if total != 5 || !slices.Equal(ids, []int64{3, 2}) {
		t.Fatalf("%v total=%d", ids, total)
	}
}

func TestListUsersByName(t *testing.T) {
	s := testStore(t)
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 1, Username: "x", FirstName: "Анна", LastName: "Кей", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	addUser(t, s, 2, "y")
	us, err := searchUsers(t, s, "анна к")
	if err != nil || len(us) != 1 || us[0].Name != "Анна Кей" || us[0].LastSeenAt == nil {
		t.Fatalf("%+v %v", us, err)
	}
}

func TestListUsersTabs(t *testing.T) {
	s := testStore(t)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 4; i++ {
		addUser(t, s, i, "u")
	}
	future := now.Add(time.Hour)
	for _, c := range []struct {
		id     int64
		exp    time.Time
		status string
	}{{1, future, "ACTIVE"}, {2, now.Add(-time.Hour), "ACTIVE"}, {3, future, "DISABLED"}} {
		if err := s.SetUserPanel(t.Context(), c.id, c.exp, c.status, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.pool.Exec(t.Context(), `UPDATE users SET blocked_bot_at = $1 WHERE telegram_id IN (1, 4)`, now); err != nil {
		t.Fatal(err)
	}
	ids, total := listIDs(t, s, model.UserFilter{Tab: model.UserTabActive, Now: now})
	if total != 1 || !slices.Equal(ids, []int64{1}) {
		t.Fatalf("active: %v total=%d", ids, total)
	}
	ids, total = listIDs(t, s, model.UserFilter{Tab: model.UserTabBlockedBot, Now: now})
	if total != 2 || !slices.Equal(ids, []int64{4, 1}) {
		t.Fatalf("blocked: %v total=%d", ids, total)
	}
}

func TestListUsersPaymentFilters(t *testing.T) {
	s := testStore(t)
	now := time.Now()
	addAdvance := func(id int64, status model.PaymentStatus) {
		t.Helper()
		if _, err := s.pool.Exec(t.Context(), `INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at, advance_granted)
			VALUES ($1, 'u', 1, 100, $2, now(), true)`, id, status); err != nil {
			t.Fatal(err)
		}
	}
	for i := int64(1); i <= 5; i++ {
		addUser(t, s, i, "u")
	}
	addPayment(t, s, 1, model.PaymentConfirmed, 100, now, &now) // только оплата
	addAdvance(2, model.PaymentDeclined)                        // долг
	addAdvance(3, model.PaymentCancelled)                       // долг погашен оплатой
	addPayment(t, s, 3, model.PaymentConfirmed, 100, now, &now)
	addPayment(t, s, 4, model.PaymentConfirmed, 100, now, &now) // долг после оплаты
	addAdvance(4, model.PaymentDeclined)
	addPayment(t, s, 5, model.PaymentDeclined, 100, now, &now) // отказ без аванса

	ids, total := listIDs(t, s, model.UserFilter{HasPayments: true, Now: now})
	if total != 3 || !slices.Equal(ids, []int64{4, 3, 1}) {
		t.Fatalf("hasPayments: %v total=%d", ids, total)
	}
	ids, total = listIDs(t, s, model.UserFilter{HasUnpaidAdvances: true, Now: now})
	if total != 2 || !slices.Equal(ids, []int64{4, 2}) {
		t.Fatalf("hasUnpaidAdvances: %v total=%d", ids, total)
	}
	// Правило то же, что у статистики админа.
	for id := int64(1); id <= 5; id++ {
		st, err := s.UserStats(t.Context(), id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := id == 2 || id == 4; (st.UnpaidAdvances > 0) != want {
			t.Fatalf("UserStats(%d) = %d", id, st.UnpaidAdvances)
		}
	}
}

func TestListUsersRegisteredPeriodAndTariff(t *testing.T) {
	s := testStore(t)
	d := func(day int) time.Time { return time.Date(2026, 10, day, 12, 0, 0, 0, time.UTC) }
	for i, day := range []int{1, 5, 10} {
		id := int64(i + 1)
		if _, err := s.pool.Exec(t.Context(), `INSERT INTO users (telegram_id, username, created_at) VALUES ($1, 'u', $2)`, id, d(day)); err != nil {
			t.Fatal(err)
		}
	}
	ids, total := listIDs(t, s, model.UserFilter{From: d(2), To: d(10), Now: d(20)})
	if total != 1 || !slices.Equal(ids, []int64{2}) {
		t.Fatalf("period: %v total=%d", ids, total)
	}

	for _, months := range []int{6, 3} {
		if _, err := s.pool.Exec(t.Context(), `INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at) VALUES (1, 'u', $1, 100, 'confirmed', now())`, months); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at) VALUES (1, 'u', 12, 100, 'declined', now())`); err != nil {
		t.Fatal(err)
	}
	u, err := s.UserByID(t.Context(), 1)
	if err != nil || u.TariffMonths == nil || *u.TariffMonths != 3 || u.Confirmed != 2 {
		t.Fatalf("tariff: %+v %v", u, err)
	}
	if u, err = s.UserByID(t.Context(), 2); err != nil || u.TariffMonths != nil {
		t.Fatalf("no tariff: %+v %v", u, err)
	}
}

func TestUserPromos(t *testing.T) {
	s := testStore(t)
	addUser(t, s, 1, "a")
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO promo_codes (code, kind, value) VALUES ('AAA', 'percent', 10), ('BBB', 'days', 5)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO promo_activations (promo_id, telegram_id)
		SELECT id, 1 FROM promo_codes ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserPromos(t.Context(), 1)
	if err != nil || len(got) != 2 || got[0].Code != "BBB" || got[0].Kind != model.PromoDays || got[0].Value != 5 || got[1].Code != "AAA" {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err = s.UserPromos(t.Context(), 2); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
}

// Код промокода приходит в платеже из join; у платежа без промокода его нет.
func TestPaymentPromoCode(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	pr, err := s.CreatePromo(ctx, model.Promo{Code: "SALE10", Kind: model.PromoPercent, Value: 10})
	if err != nil {
		t.Fatal(err)
	}
	base := 1000
	created, _, err := s.CreatePayment(ctx, model.Payment{TelegramID: 1, Month: 1, Price: 900, PrevExpireAt: time.Now(), PromoID: &pr.ID, BasePrice: &base}, noRollback)
	if err != nil {
		t.Fatal(err)
	}
	if created.PromoCode == nil || *created.PromoCode != "SALE10" {
		t.Fatalf("created = %v", created.PromoCode)
	}
	if _, _, err = s.CreatePayment(ctx, newPay(2, time.Now()), noRollback); err != nil {
		t.Fatal(err)
	}
	items, _, err := s.ListPayments(ctx, model.PaymentFilter{Limit: 10})
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v, err = %v", items, err)
	}
	if items[0].PromoCode != nil || items[1].PromoCode == nil || *items[1].PromoCode != "SALE10" || *items[1].BasePrice != 1000 {
		t.Fatalf("codes: %v %v", items[0].PromoCode, items[1].PromoCode)
	}
	// под FOR UPDATE подзапрос тоже читается
	res, err := s.ResolvePayment(ctx, created.ID, model.PaymentConfirmed, time.Now(), okApply)
	if err != nil || res.PromoCode == nil {
		t.Fatalf("resolved = %+v, err = %v", res, err)
	}
}
