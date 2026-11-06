// Package broadcast запускает запланированные рассылки: ставит их в outbox, когда подходит время.
// Доставку делает outbox-воркер; двойной запуск исключён блокировкой строки в Store.ClaimDue.
package broadcast

import (
	"context"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const tick = 20 * time.Second

type Store interface {
	ClaimDue(ctx context.Context, adminID int64, now time.Time) (int, error)
}

type Scheduler struct {
	st      Store
	adminID int64
	every   time.Duration
	now     func() time.Time
}

func New(st Store, adminID int64) *Scheduler {
	return &Scheduler{st: st, adminID: adminID, every: tick, now: time.Now}
}

// Run проверяет расписание сразу (догоняет пропущенное за время простоя) и затем каждые 20 с до отмены ctx.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		s.once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Scheduler) once(ctx context.Context) {
	n, err := s.st.ClaimDue(ctx, s.adminID, s.now())
	switch {
	case err != nil:
		if ctx.Err() == nil {
			stlog.Error(ctx, "scheduled broadcasts failed", "error", err)
		}
	case n > 0:
		stlog.Info(ctx, "scheduled broadcasts started", "count", n)
	}
}
