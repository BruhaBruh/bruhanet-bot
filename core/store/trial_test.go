package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

var errNoPanelUser = errors.New("no panel user")

// trialPanel — панель с одним юзером: создание, срок, лимит устройств; сбои по флагам.
type trialPanel struct {
	mu          sync.Mutex
	exists      bool
	expire      time.Time
	created     time.Time
	limit       int
	expireCalls int
	limitCalls  []int
	failUpdate  error
	failLimit   error
}

func (p *trialPanel) info() *api.UserItemInfo {
	return &api.UserItemInfo{ExpireAt: p.expire, CreatedAt: p.created, HwidDeviceLimit: api.NewNilInt(p.limit)}
}

func (p *trialPanel) GetUser(context.Context, int64) (*api.UserItemInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.exists {
		return nil, errNoPanelUser
	}
	return p.info(), nil
}

func (p *trialPanel) CreateUser(_ context.Context, _ int64, _ string, _ uuid.UUID) (*api.UserItemInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.exists {
		p.exists, p.expire, p.created, p.limit = true, time.Now(), time.Now(), 0
	}
	return p.info(), nil
}

func (p *trialPanel) UpdateUser(_ context.Context, _ int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	time.Sleep(20 * time.Millisecond) // окно для гонки
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failUpdate != nil {
		return nil, p.failUpdate
	}
	p.expire = expireAt
	p.expireCalls++
	return p.info(), nil
}

func (p *trialPanel) SetDeviceLimit(_ context.Context, _ int64, limit int) (*api.UserItemInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limitCalls = append(p.limitCalls, limit)
	if p.failLimit != nil && limit == 2 {
		return nil, p.failLimit
	}
	p.limit = limit
	return p.info(), nil
}

func trialReq(id int64, now time.Time) payflow.TrialRequest {
	return payflow.TrialRequest{
		TelegramID: id, Username: "u", Days: 3, Devices: 2, BaseDevices: 5, AdvanceLimit: 3, At: now,
		NotFound: func(err error) bool { return errors.Is(err, errNoPanelUser) },
	}
}

func trialGrants(t *testing.T, s *Store, id int64) (n int) {
	t.Helper()
	if err := s.pool.QueryRow(t.Context(), `SELECT count(*) FROM grants WHERE telegram_id = $1 AND source = 'trial' AND by_admin_id IS NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func trialUsedAt(t *testing.T, s *Store, id int64) (at *time.Time) {
	t.Helper()
	if err := s.pool.QueryRow(t.Context(), `SELECT trial_used_at FROM users WHERE telegram_id = $1`, id).Scan(&at); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return at
}

// seedPayments вставляет платежи юзера напрямую: статус и признак выданного аванса.
func seedPayment(t *testing.T, s *Store, id int64, status string, advance bool) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO users (telegram_id, username) VALUES ($1, '') ON CONFLICT DO NOTHING`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(t.Context(),
		`INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at, advance_granted, resolved_at)
		 VALUES ($1, 'u', 1, 150, $2, now(), $3, now())`, id, status, advance); err != nil {
		t.Fatal(err)
	}
}

func TestStartTrialEligibility(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	for name, tc := range map[string]struct {
		setup func(t *testing.T, s *Store, p *trialPanel)
		ok    bool
	}{
		"new, no panel user": {func(*testing.T, *Store, *trialPanel) {}, true},
		"expired panel user created by the bot": {func(_ *testing.T, _ *Store, p *trialPanel) {
			p.exists, p.expire, p.created = true, now.Add(-time.Hour), now.Add(-time.Hour)
		}, true},
		"declined payments below the limit": {func(t *testing.T, s *Store, _ *trialPanel) {
			seedPayment(t, s, 1, "declined", true)
			seedPayment(t, s, 1, "declined", true)
			seedPayment(t, s, 1, "cancelled", false)
		}, true},
		"blocked": {func(t *testing.T, s *Store, _ *trialPanel) {
			if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: 1, At: now}); err != nil {
				t.Fatal(err)
			}
			if err := s.SetUserBlocked(t.Context(), 1, true, now); err != nil {
				t.Fatal(err)
			}
		}, false},
		"used": {func(t *testing.T, s *Store, _ *trialPanel) {
			seedPayment(t, s, 1, "declined", false)
			if _, err := s.pool.Exec(t.Context(), `UPDATE users SET trial_used_at = $1`, now.Add(-100*day)); err != nil {
				t.Fatal(err)
			}
		}, false},
		"paid before": {func(t *testing.T, s *Store, _ *trialPanel) { seedPayment(t, s, 1, "confirmed", false) }, false},
		"active panel subscription": {func(_ *testing.T, _ *Store, p *trialPanel) {
			p.exists, p.expire, p.created = true, now.Add(5*day), now.Add(-30*day)
		}, false},
		"old customer outside the db": {func(_ *testing.T, _ *Store, p *trialPanel) {
			p.exists, p.expire, p.created = true, now.Add(-day), now.Add(-400*day)
		}, false},
		"advance limit reached": {func(t *testing.T, s *Store, _ *trialPanel) {
			for range 3 {
				seedPayment(t, s, 1, "declined", true)
			}
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			p := &trialPanel{}
			tc.setup(t, s, p)
			wasUsed := trialUsedAt(t, s, 1) != nil
			oldExpire := p.expire

			got, err := payflow.StartTrial(t.Context(), s, p, trialReq(1, now))
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				if !errors.Is(err, model.ErrTrialUnavailable) {
					t.Fatalf("err = %v", err)
				}
				if p.expireCalls != 0 || len(p.limitCalls) != 0 || !p.expire.Equal(oldExpire) {
					t.Fatalf("panel touched: expire calls %d limits %v", p.expireCalls, p.limitCalls)
				}
				if trialGrants(t, s, 1) != 0 || len(outboxTypes(t, s)) != 0 || (trialUsedAt(t, s, 1) != nil) != wasUsed {
					t.Fatal("refusal left traces")
				}
				return
			}
			want := now.Add(3 * day)
			if !got.Equal(want) || !p.expire.Equal(want) || p.limit != 2 {
				t.Fatalf("expire %v panel %v limit %d", got, p.expire, p.limit)
			}
			if at := trialUsedAt(t, s, 1); at == nil || !at.Equal(now) || trialGrants(t, s, 1) != 1 {
				t.Fatalf("trial_used_at %v grants %d", at, trialGrants(t, s, 1))
			}
			items, _ := s.OutboxBatch(t.Context(), 10)
			if len(items) != 1 || items[0].EventType != payflow.EventTrial || items[0].TextKey != "trial.started" ||
				items[0].Days != 3 || items[0].Devices != 2 || !items[0].ExpireAt.Equal(want) {
				t.Fatalf("outbox %+v", items)
			}
			// карточка «Выдано админом» пробный не показывает
			if gs, _ := s.UserGrants(t.Context(), 1, 20); len(gs) != 0 {
				t.Fatalf("admin grants list has the trial: %+v", gs)
			}
			// повтор — отказ, ничего не добавлено
			if _, err = payflow.StartTrial(t.Context(), s, p, trialReq(1, now.Add(time.Minute))); !errors.Is(err, model.ErrTrialUnavailable) || p.expireCalls != 1 {
				t.Fatalf("repeat: err %v calls %d", err, p.expireCalls)
			}
		})
	}
}

func TestStartTrialRaceGivesOne(t *testing.T) {
	s := testStore(t)
	p := &trialPanel{}
	now := time.Now().Truncate(time.Microsecond)
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() { _, errs[i] = payflow.StartTrial(t.Context(), s, p, trialReq(1, now)) })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, model.ErrTrialUnavailable):
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 || p.expireCalls != 1 || trialGrants(t, s, 1) != 1 || len(outboxTypes(t, s)) != 1 {
		t.Fatalf("ok=%d expire calls=%d grants=%d outbox=%d", ok, p.expireCalls, trialGrants(t, s, 1), len(outboxTypes(t, s)))
	}
}

func TestStartTrialPanelFailure(t *testing.T) {
	now := time.Now().Truncate(time.Microsecond)
	old := now.Add(-time.Hour)

	t.Run("expire write fails", func(t *testing.T) {
		s := testStore(t)
		p := &trialPanel{exists: true, expire: old, created: old, failUpdate: errors.New("down")}
		if _, err := payflow.StartTrial(t.Context(), s, p, trialReq(1, now)); !errors.Is(err, payflow.ErrPanel) {
			t.Fatalf("err = %v", err)
		}
		if trialUsedAt(t, s, 1) != nil || trialGrants(t, s, 1) != 0 || len(outboxTypes(t, s)) != 0 || !p.expire.Equal(old) {
			t.Fatal("rollback left traces")
		}
		// повтор считает ту же дату
		p.failUpdate = nil
		got, err := payflow.StartTrial(t.Context(), s, p, trialReq(1, now))
		if err != nil || !got.Equal(now.Add(3*day)) {
			t.Fatalf("retry %v %v", got, err)
		}
	})

	t.Run("limit write fails after the expire write: expire and limit are restored", func(t *testing.T) {
		s := testStore(t)
		p := &trialPanel{exists: true, expire: old, created: old, limit: 5, failLimit: errors.New("down")}
		_, err := payflow.StartTrial(t.Context(), s, p, trialReq(1, now))
		if !errors.Is(err, payflow.ErrPanel) || errors.Is(err, payflow.ErrNotRestored) {
			t.Fatalf("err = %v", err)
		}
		if !p.expire.Equal(old) || p.limit != 5 {
			t.Fatalf("panel not restored: expire %v limit %d", p.expire, p.limit)
		}
		if trialUsedAt(t, s, 1) != nil || trialGrants(t, s, 1) != 0 || len(outboxTypes(t, s)) != 0 {
			t.Fatal("rollback left traces")
		}
	})
}

// Пробный = один неоплаченный аванс; подтверждённая подписка сбрасывает его и пишет в панель base+extra один раз.
func TestTrialIsOneUnpaidAdvanceAndConvertsOnce(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	p := &trialPanel{}
	if _, err := payflow.StartTrial(ctx, s, p, trialReq(1, now)); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.UserStats(ctx, 1, 0); st.UnpaidAdvances != 1 {
		t.Fatalf("stats after trial: %d", st.UnpaidAdvances)
	}
	seedPayment(t, s, 1, "declined", true)
	if st, _ := s.UserStats(ctx, 1, 0); st.UnpaidAdvances != 2 {
		t.Fatalf("stats with a declined advance: %d", st.UnpaidAdvances)
	}
	if ok, _ := s.TrialAllowed(ctx, 1, 3); ok {
		t.Fatal("trial allowed again")
	}

	// оплата: срок от конца пробного, лимит base+extra
	pay, _, err := s.CreatePayment(ctx, newPay(1, now.Add(3*day)), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	limits := len(p.limitCalls)
	if _, err = payflow.Resolve(ctx, s, p, payflow.Decision{ID: pay.ID, Approve: true, At: now.Add(time.Hour), DevicesBase: 5}); err != nil {
		t.Fatal(err)
	}
	if len(p.limitCalls) != limits+1 || p.limit != 5 {
		t.Fatalf("conversion: limit %d calls %v", p.limit, p.limitCalls)
	}
	if st, _ := s.UserStats(ctx, 1, 0); st.UnpaidAdvances != 0 { // и пробный, и declined-аванс сброшены оплатой
		t.Fatalf("stats after payment: %d", st.UnpaidAdvances)
	}
	// следующая подписка лимит не трогает
	pay2, _, err := s.CreatePayment(ctx, newPay(1, now.Add(40*day)), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payflow.Resolve(ctx, s, p, payflow.Decision{ID: pay2.ID, Approve: true, At: now.Add(2 * time.Hour), DevicesBase: 5}); err != nil {
		t.Fatal(err)
	}
	if len(p.limitCalls) != limits+1 {
		t.Fatalf("second payment wrote the limit: %v", p.limitCalls)
	}
	// отклонённая подписка пробного юзера лимит тоже не пишет
	s2 := testStore(t)
	p2 := &trialPanel{}
	if _, err = payflow.StartTrial(ctx, s2, p2, trialReq(1, now)); err != nil {
		t.Fatal(err)
	}
	pay3, _, _ := s2.CreatePayment(ctx, newPay(1, now.Add(3*day)), noRollback, nil)
	n := len(p2.limitCalls)
	if _, err = payflow.Resolve(ctx, s2, p2, payflow.Decision{ID: pay3.ID, Approve: false, At: now, DevicesBase: 5}); err != nil || len(p2.limitCalls) != n {
		t.Fatalf("decline: %v %v", err, p2.limitCalls)
	}
}

// Пробный при открытой оплате сдвигает базу pending на те же дни.
func TestStartTrialShiftsPending(t *testing.T) {
	s := testStore(t)
	now := time.Now().Truncate(time.Microsecond)
	if _, _, err := s.CreatePayment(t.Context(), newPay(1, now), noRollback, nil); err != nil {
		t.Fatal(err)
	}
	p := &trialPanel{exists: true, expire: now, created: now}
	if _, err := payflow.StartTrial(t.Context(), s, p, trialReq(1, now)); err != nil {
		t.Fatal(err)
	}
	if got := pendingPrev(t, s, 1); !got.Equal(now.Add(3 * day)) {
		t.Fatalf("pending prev = %v", got)
	}
}

// Аудитория «Пробный период» и вкладка/признак в списке юзеров считают одинаково: взявшие пробный и не оплатившие подписку.
func TestTrialAudienceTabAndFlag(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	for id := int64(1); id <= 5; id++ {
		if err := s.TouchUser(ctx, model.UserSeen{TelegramID: id, At: now}); err != nil {
			t.Fatal(err)
		}
	}
	// 1, 2, 3 взяли пробный; 2 заплатил подпиской; 3 заблокировал бота; 4 платил, пробного не брал; 5 — никто
	if _, err := s.pool.Exec(ctx, `UPDATE users SET trial_used_at = $1 WHERE telegram_id IN (1, 2, 3)`, now); err != nil {
		t.Fatal(err)
	}
	seedPayment(t, s, 2, "confirmed", false)
	seedPayment(t, s, 4, "confirmed", false)
	if err := s.MarkBotBlocked(ctx, 3); err != nil {
		t.Fatal(err)
	}
	// подтверждённая докупка устройств подписку не оплачивает
	if _, err := s.pool.Exec(ctx, `INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at, kind, extra_devices, resolved_at)
		VALUES (1, 'u', 0, 50, 'confirmed', now(), 'devices', 1, now())`); err != nil {
		t.Fatal(err)
	}

	c, err := s.AudienceCounts(ctx, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := recipients(ctx, s.pool, broadcast.AudienceTrial, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.Trial != 1 || len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("count %d recipients %v (blocked bot excluded, paid excluded)", c.Trial, ids)
	}

	us, total, err := s.ListUsers(ctx, model.UserFilter{Tab: model.UserTabTrial, Now: now, Limit: 50})
	if err != nil || total != 2 || len(us) != 2 { // вкладка показывает и заблокировавших бота
		t.Fatalf("tab: %d %+v %v", total, us, err)
	}
	for _, u := range us {
		if !u.InTrial || u.TrialUsedAt == nil {
			t.Fatalf("flag: %+v", u)
		}
	}
	u2, _ := s.UserByID(ctx, 2)
	if u2.InTrial || u2.TrialUsedAt == nil {
		t.Fatalf("paid user: %+v", u2)
	}
	// «Есть неоплаченные авансы» включает пробный
	us, total, err = s.ListUsers(ctx, model.UserFilter{HasUnpaidAdvances: true, Now: now, Limit: 50})
	if err != nil || total != 2 {
		t.Fatalf("unpaid advances filter: %d %+v %v", total, us, err)
	}
}
