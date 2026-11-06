// Package referral — сверка наград рефералки: дни пригласившему выдаются сразу после подтверждения первой оплаты друга,
// а всё, что не вышло (сбой панели, падение между коммитом платежа и выдачей, блокировка пригласившего), добирается здесь.
package referral

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	batch       = 20
	itemTimeout = 20 * time.Second
	backoffBase = 5 * time.Minute
	backoffMax  = 6 * time.Hour
)

// Store — core/store.
type Store interface {
	payflow.ReferralStore
	ReferralsDue(ctx context.Context, skip []int64, limit int) ([]int64, error)
}

type failure struct {
	n     int
	until time.Time
}

// Reconciler платит пригласившим по наградам без referrer_rewarded_at. Повторяющийся сбой одной награды откладывается
// с растущей паузой (счётчик в памяти процесса, как у ClaimDue): остальные строки не голодают.
type Reconciler struct {
	st    Store
	panel payflow.GrantPanel
	squad uuid.UUID
	now   func() time.Time

	mu    sync.Mutex
	fails map[int64]failure
}

func New(st Store, panel payflow.GrantPanel, squad uuid.UUID, now func() time.Time) *Reconciler {
	if now == nil {
		now = time.Now
	}
	return &Reconciler{st: st, panel: panel, squad: squad, now: now, fails: map[int64]failure{}}
}

// Run — один проход: до batch наград. Параллельный запуск (два бота, ручной вызов) безопасен: выдача идемпотентна под замком.
func (r *Reconciler) Run(ctx context.Context) {
	at := r.now()
	ids, err := r.st.ReferralsDue(ctx, r.skip(at), batch)
	if err != nil {
		stlog.Error(ctx, "failed to list referral rewards", "error", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		r.pay(ctx, id, at)
	}
}

func (r *Reconciler) skip(at time.Time) []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []int64{}
	for id, f := range r.fails {
		if f.until.After(at) {
			out = append(out, id)
		}
	}
	return out
}

func (r *Reconciler) pay(ctx context.Context, referredID int64, at time.Time) {
	ictx, cancel := context.WithTimeout(ctx, itemTimeout)
	defer cancel()
	_, err := payflow.PayReferrer(ictx, r.st, r.panel, referredID, at, r.squad)
	switch {
	case err == nil:
		stlog.Info(ctx, "referrer rewarded", "referred_id", referredID)
		r.forget(referredID)
	case errors.Is(err, store.ErrAlreadyRewarded), errors.Is(err, store.ErrNotFound):
		r.forget(referredID)
	case errors.Is(err, store.ErrBlocked):
		// награда остаётся: ReferralsDue вернёт её после снятия блокировки
		stlog.Info(ctx, "referrer bonus skipped: referrer is blocked", "referred_id", referredID)
	default:
		n := r.fail(referredID, at)
		stlog.Error(ctx, "failed to reward referrer", "referred_id", referredID, "attempt", n, "error", err)
	}
}

func (r *Reconciler) forget(id int64) {
	r.mu.Lock()
	delete(r.fails, id)
	r.mu.Unlock()
}

func (r *Reconciler) fail(id int64, at time.Time) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.fails[id]
	f.n++
	d := backoffBase << min(f.n-1, 10)
	f.until = at.Add(min(d, backoffMax))
	r.fails[id] = f
	return f.n
}
