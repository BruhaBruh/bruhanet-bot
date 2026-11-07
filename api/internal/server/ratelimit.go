package server

import (
	"sync"
	"time"
)

// limiter — скользящее окно по ключу, в памяти процесса.
// ponytail: один инстанс api; при нескольких — вынести в Redis.
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}} }

// allow считает вызов и возвращает false, если за window уже было n вызовов.
func (l *limiter) allow(key string, n int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := now.Add(-window)
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= n {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
