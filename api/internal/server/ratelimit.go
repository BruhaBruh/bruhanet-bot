package server

import (
	"sync"
	"time"
)

const (
	limiterSweepAt   = 4096
	limiterMaxWindow = time.Hour // самое длинное окно среди лимитов приложения
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
	if len(l.hits) > limiterSweepAt { // ключи по юзерам копятся: сбрасываем тех, кто молчит дольше самого длинного окна
		for k, v := range l.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > limiterMaxWindow {
				delete(l.hits, k)
			}
		}
	}
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

// seenSet запоминает ключи на ttl: одноразовые подписи. Просроченные выбрасываются при обращении.
type seenSet struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func newSeenSet() *seenSet { return &seenSet{m: map[string]time.Time{}} }

// first отмечает key и возвращает true, если за ttl его ещё не видели.
func (s *seenSet) first(key string, ttl time.Duration, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, at := range s.m {
		if now.Sub(at) >= ttl {
			delete(s.m, k)
		}
	}
	if _, dup := s.m[key]; dup {
		return false
	}
	s.m[key] = now
	return true
}
