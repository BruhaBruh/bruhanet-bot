package observability

import (
	"context"
	"maps"
	"sync"
	"time"
)

// Check проверяет одну зависимость; nil = жива. Обязана учитывать ctx (таймаут и отмена).
type Check func(ctx context.Context) error

type Readiness struct {
	checks   map[string]Check
	interval time.Duration
	timeout  time.Duration

	mu   sync.RWMutex
	done bool
	deps map[string]bool
}

// NewReadiness: до первого прохода Run всё считается down.
func NewReadiness(checks map[string]Check, interval, timeout time.Duration) *Readiness {
	Ready.Set(0)
	for name := range checks {
		DependencyUp.WithLabelValues(name).Set(0)
	}
	return &Readiness{checks: checks, interval: interval, timeout: timeout, deps: map[string]bool{}}
}

// Run проверяет сразу, затем по тикеру, пока ctx не отменён.
func (r *Readiness) Run(ctx context.Context) {
	r.check(ctx)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.check(ctx)
		}
	}
}

func (r *Readiness) check(ctx context.Context) {
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		res = make(map[string]bool, len(r.checks))
	)
	for name, c := range r.checks {
		wg.Go(func() {
			cctx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			up := c(cctx) == nil && cctx.Err() == nil
			mu.Lock()
			res[name] = up
			mu.Unlock()
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return // остановка: результат недостоверен
	}

	all := true
	for name, up := range res {
		v := 0.0
		if up {
			v = 1
		} else {
			all = false
		}
		DependencyUp.WithLabelValues(name).Set(v)
	}
	if all {
		Ready.Set(1)
	} else {
		Ready.Set(0)
	}
	r.mu.Lock()
	r.done, r.deps = true, res
	r.mu.Unlock()
}

// Status: ready=false до первого прохода и при любой упавшей зависимости.
func (r *Readiness) Status() (bool, map[string]bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	deps := maps.Clone(r.deps)
	if !r.done {
		return false, deps
	}
	for _, up := range deps {
		if !up {
			return false, deps
		}
	}
	return true, deps
}
