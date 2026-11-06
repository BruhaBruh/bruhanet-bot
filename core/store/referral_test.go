package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

const refDays = 7

// refPanel — панель на несколько юзеров: срок и записи по каждому, запись с задержкой.
type refPanel struct {
	mu      sync.Mutex
	expire  map[int64]time.Time
	writes  map[int64][]time.Time
	failFor map[int64]error
}

func newRefPanel() *refPanel {
	return &refPanel{expire: map[int64]time.Time{}, writes: map[int64][]time.Time{}, failFor: map[int64]error{}}
}

func (p *refPanel) UpdateUser(_ context.Context, id int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	time.Sleep(40 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.failFor[id]; err != nil {
		return nil, err
	}
	p.expire[id] = expireAt
	p.writes[id] = append(p.writes[id], expireAt)
	return &api.UserItemInfo{ExpireAt: expireAt}, nil
}

func (p *refPanel) SetDeviceLimit(context.Context, int64, int) (*api.UserItemInfo, error) {
	return &api.UserItemInfo{}, nil
}

func (p *refPanel) CreateUser(ctx context.Context, id int64, _ string, _ uuid.UUID) (*api.UserItemInfo, error) {
	return p.GetUser(ctx, id)
}

func (p *refPanel) GetUser(_ context.Context, id int64) (*api.UserItemInfo, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return &api.UserItemInfo{ExpireAt: p.expire[id]}, nil
}

func (p *refPanel) written(id int64) []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time{}, p.writes[id]...)
}

func (p *refPanel) setFail(id int64, err error) {
	p.mu.Lock()
	p.failFor[id] = err
	p.mu.Unlock()
}

type refFixture struct {
	s        *Store
	panel    *refPanel
	now      time.Time
	referrer int64
	friend   int64
}

// newRefFixture: пригласивший 10 уже в users, друг 20 пришёл по его ссылке.
func newRefFixture(t *testing.T) *refFixture {
	t.Helper()
	f := &refFixture{s: testStore(t), panel: newRefPanel(), now: time.Now().Truncate(time.Microsecond), referrer: 10, friend: 20}
	ctx := t.Context()
	if err := f.s.TouchUser(ctx, model.UserSeen{TelegramID: f.referrer, Username: "ref", At: f.now}); err != nil {
		t.Fatal(err)
	}
	ok, err := f.s.RecordReferral(ctx, model.UserSeen{TelegramID: f.friend, Username: "friend", At: f.now}, f.referrer)
	if err != nil || !ok {
		t.Fatalf("RecordReferral = %v, %v", ok, err)
	}
	return f
}

// pending создаёт и «проверенный» pending-платёж юзера.
func (f *refFixture) pending(t *testing.T, id int64, kind model.PaymentKind, price int, prev time.Time) model.Payment {
	t.Helper()
	p := model.Payment{TelegramID: id, Username: "u", Month: 1, Price: price, PrevExpireAt: prev, Kind: kind}
	if kind == model.KindDevices {
		p.Month, p.ExtraDevices = 0, 1
	}
	created, _, err := f.s.CreatePayment(t.Context(), p, noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func (f *refFixture) resolve(ctx context.Context, id int64, approve bool, days int) (model.Payment, error) {
	return payflow.Resolve(ctx, f.s, f.panel, payflow.Decision{ID: id, Approve: approve, At: f.now, DevicesBase: 5, ReferralDays: days})
}

func (f *refFixture) count(t *testing.T, q string, args ...any) (n int) {
	t.Helper()
	if err := f.s.pool.QueryRow(t.Context(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *refFixture) rewards(t *testing.T) int {
	return f.count(t, `SELECT count(*) FROM referral_rewards`)
}

func (f *refFixture) grants(t *testing.T, id int64) int {
	return f.count(t, `SELECT count(*) FROM grants WHERE telegram_id = $1 AND source = 'referral'`, id)
}

func (f *refFixture) outboxEvents(t *testing.T, chat int64) (n int) {
	return f.count(t, `SELECT count(*) FROM outbox WHERE event_type = ANY('{referral_friend,referral_referrer}') AND (payload->>'chat_id')::bigint = $1`, chat)
}

func TestRecordReferral(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now()
	for _, id := range []int64{10, 11} {
		if err := s.TouchUser(ctx, model.UserSeen{TelegramID: id, Username: "r", At: now}); err != nil {
			t.Fatal(err)
		}
	}
	referredBy := func(id int64) *int64 {
		t.Helper()
		r, err := s.UserReferral(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return r.ReferredBy
	}
	rec := func(id, ref int64) bool {
		t.Helper()
		ok, err := s.RecordReferral(ctx, model.UserSeen{TelegramID: id, Username: "u", At: now}, ref)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	if !rec(20, 10) || referredBy(20) == nil || *referredBy(20) != 10 {
		t.Fatal("первый старт по ссылке не записан")
	}
	if rec(20, 11) || *referredBy(20) != 10 {
		t.Fatal("второй старт с другим payload перезаписал пригласившего")
	}
	// LastSeen позже обновляет строку, но не пригласившего
	if err := s.TouchUser(ctx, model.UserSeen{TelegramID: 20, Username: "u2", At: now}); err != nil || *referredBy(20) != 10 {
		t.Fatalf("TouchUser затёр referred_by: %v", err)
	}
	if rec(21, 21) {
		t.Fatal("реферал сам на себя записан")
	}
	if rec(22, 999) {
		t.Fatal("несуществующий пригласивший записан")
	}
	// отказ не создаёт строку: следующий обычный касание заведёт юзера без пригласившего
	if n := func() (n int) {
		_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE telegram_id IN (21, 22)`).Scan(&n)
		return n
	}(); n != 0 {
		t.Fatalf("отказ создал %d строк юзера", n)
	}
	if rec(10, 11) { // уже существующий юзер не может стать чьим-то приглашённым
		t.Fatal("существующий юзер получил пригласившего")
	}
	if referredBy(10) != nil {
		t.Fatal("referred_by существующего юзера записан")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET referred_by = telegram_id WHERE telegram_id = 11`); err == nil {
		t.Fatal("CHECK referred_by <> telegram_id не сработал")
	}
	r, err := s.UserReferral(ctx, 10)
	if err != nil || r.Invited != 1 || r.Paid != 0 {
		t.Fatalf("карточка пригласившего %+v, %v", r, err)
	}
	if r, _ = s.UserReferral(ctx, 20); r.ReferredBy == nil || r.ReferredByUsername != "r" {
		t.Fatalf("карточка друга %+v", r)
	}
}

// Двойное подтверждение (бот и панель) и повтор: награда одна, друг получает бонус одной записью в панель.
func TestReferralRewardExactlyOnce(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	prev := f.now.Add(48 * time.Hour).Truncate(time.Microsecond)
	p := f.pending(t, f.friend, model.KindTariff, 150, prev)

	errs := make([]error, 3)
	pays := make([]model.Payment, 3)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pays[i], errs[i] = f.resolve(ctx, p.ID, true, refDays)
		}()
	}
	wg.Wait()

	won := -1
	for i, err := range errs {
		switch {
		case err == nil && won < 0:
			won = i
		case errors.Is(err, ErrNotPending):
		default:
			t.Fatalf("resolve %d: %v", i, err)
		}
	}
	if won < 0 || !pays[won].ReferralPending {
		t.Fatalf("победитель %d, pending=%v", won, pays[won].ReferralPending)
	}
	want := payflow.ExtendedExpire(prev, f.now, 1).Add(refDays * 24 * time.Hour)
	if w := f.panel.written(f.friend); len(w) != 1 || !w[0].Equal(want) {
		t.Fatalf("запись срока друга %v, want одна %v", w, want)
	}
	if f.rewards(t) != 1 || f.grants(t, f.friend) != 1 || f.outboxEvents(t, f.friend) != 1 {
		t.Fatalf("rewards=%d grants=%d outbox=%d", f.rewards(t), f.grants(t, f.friend), f.outboxEvents(t, f.friend))
	}
	if f.count(t, `SELECT count(*) FROM referral_rewards WHERE referred_rewarded_at IS NOT NULL AND referrer_rewarded_at IS NULL`) != 1 {
		t.Fatal("отметка друга не стоит")
	}
	// повтор подтверждения
	if _, err := f.resolve(ctx, p.ID, true, refDays); !errors.Is(err, ErrNotPending) {
		t.Fatalf("повтор: %v", err)
	}

	// пригласивший: параллельные запуски (немедленный шаг в боте и api + сверка) платят один раз
	f.panel.expire[f.referrer] = f.now.Add(24 * time.Hour)
	prevRef := f.now.Add(24 * time.Hour)
	perrs := make([]error, 4)
	for i := range perrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, perrs[i] = payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range perrs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyRewarded):
		default:
			t.Fatalf("PayReferrer: %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("успешных выдач %d, want 1", ok)
	}
	if w := f.panel.written(f.referrer); len(w) != 1 || !w[0].Equal(prevRef.Add(refDays*24*time.Hour)) {
		t.Fatalf("запись срока пригласившего %v", w)
	}
	if f.grants(t, f.referrer) != 1 || f.outboxEvents(t, f.referrer) != 1 {
		t.Fatalf("grants=%d outbox=%d", f.grants(t, f.referrer), f.outboxEvents(t, f.referrer))
	}
	if _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); !errors.Is(err, ErrAlreadyRewarded) {
		t.Fatalf("повтор PayReferrer: %v", err)
	}
	if r, _ := f.s.UserReferral(ctx, f.referrer); r.Invited != 1 || r.Paid != 1 {
		t.Fatalf("карточка %+v", r)
	}

	// вторая оплата друга награды не даёт
	p2 := f.pending(t, f.friend, model.KindTariff, 150, prev)
	if res, err := f.resolve(ctx, p2.ID, true, refDays); err != nil || res.ReferralPending {
		t.Fatalf("вторая оплата: pending=%v err=%v", res.ReferralPending, err)
	}
	if f.rewards(t) != 1 || f.grants(t, f.friend) != 1 {
		t.Fatal("вторая оплата выдала награду")
	}
}

func TestReferralNotTriggered(t *testing.T) {
	for name, tc := range map[string]struct {
		kind    model.PaymentKind
		price   int
		approve bool
		days    int
	}{
		"price 0":          {model.KindTariff, 0, true, refDays},
		"devices":          {model.KindDevices, 50, true, refDays},
		"declined":         {model.KindTariff, 150, false, refDays},
		"referral off":     {model.KindTariff, 150, true, 0},
		"declined devices": {model.KindDevices, 50, false, refDays},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRefFixture(t)
			p := f.pending(t, f.friend, tc.kind, tc.price, f.now)
			res, err := f.resolve(t.Context(), p.ID, tc.approve, tc.days)
			if err != nil || res.ReferralPending {
				t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
			}
			if f.rewards(t) != 0 || f.grants(t, f.friend) != 0 || f.outboxEvents(t, f.friend) != 0 {
				t.Fatal("награда создана")
			}
		})
	}
}

func TestReferralCancelledPaymentNoReward(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	// новая оплата отменяет прежнюю pending; отменённая не подтверждается
	p2 := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if _, err := f.resolve(ctx, p.ID, true, refDays); !errors.Is(err, ErrNotPending) {
		t.Fatalf("подтверждение отменённого: %v", err)
	}
	if f.rewards(t) != 0 {
		t.Fatal("отменённый платёж дал награду")
	}
	if res, err := f.resolve(ctx, p2.ID, true, refDays); err != nil || !res.ReferralPending {
		t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
	}
	if f.rewards(t) != 1 {
		t.Fatal("актуальный платёж не дал награду")
	}
}

// Первое подтверждение без рефералки закрывает окно: награда за вторую оплату не выдаётся.
func TestReferralOnlyFirstPaidConfirmation(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	first := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if _, err := f.resolve(ctx, first.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	second := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if res, err := f.resolve(ctx, second.ID, true, refDays); err != nil || res.ReferralPending || f.rewards(t) != 0 {
		t.Fatalf("pending=%v err=%v rewards=%d", res.ReferralPending, err, f.rewards(t))
	}
}

// Оплата 0 ₽ (промокод) подтверждается мимо рефералки; первая платная после неё всё равно награждается.
func TestReferralFreePaymentThenPaid(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	free := f.pending(t, f.friend, model.KindTariff, 0, f.now)
	if _, err := f.s.ResolvePayment(ctx, free.ID, model.PaymentConfirmed, f.now, noApply); err != nil {
		t.Fatal(err)
	}
	paid := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	res, err := f.resolve(ctx, paid.ID, true, refDays)
	if err != nil || !res.ReferralPending {
		t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
	}
}

func TestReferralDevicesFirstThenTariff(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	dev := f.pending(t, f.friend, model.KindDevices, 50, f.now)
	if _, err := f.resolve(ctx, dev.ID, true, refDays); err != nil || f.rewards(t) != 0 {
		t.Fatalf("докупка: %v rewards=%d", err, f.rewards(t))
	}
	tariff := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if res, err := f.resolve(ctx, tariff.ID, true, refDays); err != nil || !res.ReferralPending {
		t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
	}
}

func TestReferralTrialThenFirstTariff(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	if _, err := f.s.pool.Exec(ctx, `UPDATE users SET trial_used_at = $2 WHERE telegram_id = $1`, f.friend, f.now); err != nil {
		t.Fatal(err)
	}
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now.Add(3*24*time.Hour))
	res, err := f.resolve(ctx, p.ID, true, refDays)
	if err != nil || !res.ReferralPending {
		t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
	}
	if w := f.panel.written(f.friend); len(w) != 1 || !w[0].Equal(payflow.ExtendedExpire(p.PrevExpireAt, f.now, 1).Add(refDays*24*time.Hour)) {
		t.Fatalf("запись %v", w)
	}
}

// Заблокированный друг платит как обычно: себе дней не получает, пригласивший получает.
func TestReferralBlockedFriendStillRewardsReferrer(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	if err := f.s.SetUserBlocked(ctx, f.friend, true, f.now); err != nil {
		t.Fatal(err)
	}
	prev := f.now.Add(24 * time.Hour)
	p := f.pending(t, f.friend, model.KindTariff, 150, prev)
	res, err := f.resolve(ctx, p.ID, true, refDays)
	if err != nil || !res.ReferralPending {
		t.Fatalf("pending=%v err=%v", res.ReferralPending, err)
	}
	if w := f.panel.written(f.friend); len(w) != 1 || !w[0].Equal(payflow.ExtendedExpire(prev, f.now, 1)) {
		t.Fatalf("друг получил бонус: %v", w)
	}
	if f.grants(t, f.friend) != 0 || f.outboxEvents(t, f.friend) != 0 {
		t.Fatal("бонус или уведомление заблокированному другу")
	}
	if f.count(t, `SELECT count(*) FROM referral_rewards WHERE referred_rewarded_at IS NULL`) != 1 {
		t.Fatal("referred_rewarded_at должен остаться NULL")
	}
	if _, err = payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); err != nil {
		t.Fatalf("пригласивший: %v", err)
	}
}

func TestReferralBlockedReferrerSkippedThenPaidAfterUnblock(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if _, err := f.resolve(ctx, p.ID, true, refDays); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SetUserBlocked(ctx, f.referrer, true, f.now); err != nil {
		t.Fatal(err)
	}
	if _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("заблокированный пригласивший: %v", err)
	}
	if len(f.panel.written(f.referrer)) != 0 || f.grants(t, f.referrer) != 0 {
		t.Fatal("заблокированному выданы дни")
	}
	if due, err := f.s.ReferralsDue(ctx, nil, 10); err != nil || len(due) != 0 {
		t.Fatalf("due при блокировке: %v %v", due, err)
	}
	if err := f.s.SetUserBlocked(ctx, f.referrer, false, f.now); err != nil {
		t.Fatal(err)
	}
	due, err := f.s.ReferralsDue(ctx, nil, 10)
	if err != nil || len(due) != 1 || due[0] != f.friend {
		t.Fatalf("due после снятия: %v %v", due, err)
	}
	if skipped, _ := f.s.ReferralsDue(ctx, []int64{f.friend}, 10); len(skipped) != 0 {
		t.Fatal("skip не работает")
	}
	if _, err = payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if f.grants(t, f.referrer) != 1 {
		t.Fatal("после снятия блокировки дни не выданы")
	}
	if due, _ = f.s.ReferralsDue(ctx, nil, 10); len(due) != 0 {
		t.Fatal("выданная награда осталась в due")
	}
}

func TestReferralPanelFailureLeavesRetryable(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if _, err := f.resolve(ctx, p.ID, true, refDays); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("panel down")
	f.panel.setFail(f.referrer, boom)
	if _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if f.grants(t, f.referrer) != 0 || f.outboxEvents(t, f.referrer) != 0 {
		t.Fatal("сбой панели оставил следы")
	}
	if due, _ := f.s.ReferralsDue(ctx, nil, 10); len(due) != 1 {
		t.Fatal("награда не осталась к повтору")
	}
	f.panel.setFail(f.referrer, nil)
	got, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil)
	if err != nil || !got.Equal(f.now.Add(refDays*24*time.Hour)) {
		t.Fatalf("повтор: %v %v", got, err)
	}
}

// Сбой панели друга откатывает подтверждение целиком: награды нет, платёж pending; повтор считает ту же дату.
func TestReferralFriendPanelFailureRollsBack(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	f.panel.setFail(f.friend, errors.New("down"))
	if _, err := f.resolve(ctx, p.ID, true, refDays); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if f.rewards(t) != 0 || f.grants(t, f.friend) != 0 {
		t.Fatal("откат оставил награду")
	}
	f.panel.setFail(f.friend, nil)
	if res, err := f.resolve(ctx, p.ID, true, refDays); err != nil || !res.ReferralPending {
		t.Fatalf("повтор: pending=%v err=%v", res.ReferralPending, err)
	}
}

// Выдача пригласившему сдвигает базу его pending-платежа: подтверждение не затрёт бонус.
func TestReferralShiftsReferrerPending(t *testing.T) {
	f := newRefFixture(t)
	ctx := t.Context()
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	if _, err := f.resolve(ctx, p.ID, true, refDays); err != nil {
		t.Fatal(err)
	}
	prev := f.now.Add(10 * 24 * time.Hour).Truncate(time.Microsecond)
	f.panel.expire[f.referrer] = prev
	own := f.pending(t, f.referrer, model.KindTariff, 150, prev)
	if _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if got, want := pendingPrev(t, f.s, f.referrer), prev.Add(refDays*24*time.Hour); !got.Equal(want) {
		t.Fatalf("prev_expire_at %v, want %v", got, want)
	}
	_ = own
}

// Пригласивший оплачивает своё, друг подтверждается, идёт выдача и сверка: без дедлока, итог детерминирован.
func TestReferralNoDeadlockWithReferrerOwnPayments(t *testing.T) {
	f := newRefFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	own := f.pending(t, f.referrer, model.KindTariff, 150, f.now)
	p := f.pending(t, f.friend, model.KindTariff, 150, f.now)
	var wg sync.WaitGroup
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && !errors.Is(err, ErrNotPending) && !errors.Is(err, ErrAlreadyRewarded) && !errors.Is(err, ErrNotFound) {
				t.Errorf("%v", err)
			}
		}()
	}
	run(func() error { _, err := f.resolve(ctx, own.ID, true, refDays); return err })
	run(func() error { _, err := f.resolve(ctx, p.ID, true, refDays); return err })
	run(func() error { _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); return err })
	run(func() error {
		_, _, err := f.s.CreatePayment(ctx, model.Payment{TelegramID: f.referrer, Username: "u", Month: 3, Price: 400, PrevExpireAt: f.now}, noRollback, nil)
		return err
	})
	wg.Wait()
	if ctx.Err() != nil {
		t.Fatal("таймаут: вероятный дедлок")
	}
	if _, err := payflow.PayReferrer(ctx, f.s, f.panel, f.friend, f.now, uuid.Nil); err != nil && !errors.Is(err, ErrAlreadyRewarded) && !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if f.grants(t, f.referrer) > 1 {
		t.Fatal("пригласивший награждён дважды")
	}
}
