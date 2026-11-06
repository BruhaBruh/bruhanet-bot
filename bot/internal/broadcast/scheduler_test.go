package broadcast

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeStore struct {
	calls atomic.Int32
	admin atomic.Int64
	err   error
}

func (f *fakeStore) ClaimDue(_ context.Context, adminID int64, _ time.Time) (int, error) {
	f.calls.Add(1)
	f.admin.Store(adminID)
	return 0, f.err
}

func TestRunChecksImmediatelyThenOnTick(t *testing.T) {
	st := &fakeStore{err: errors.New("boom")} // ошибка не останавливает цикл
	s := New(st, 42)
	s.every = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for st.calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("calls = %d", st.calls.Load())
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
	if st.admin.Load() != 42 {
		t.Fatalf("admin = %d", st.admin.Load())
	}
}
