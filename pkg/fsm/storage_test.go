package fsm_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func testStorage(t *testing.T, newStore func(t *testing.T) fsm.Storage) {
	t.Helper()
	ctx := context.Background()

	t.Run("get missing returns empty record", func(t *testing.T) {
		s := newStore(t)
		r, err := s.Get(ctx, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if r.State != fsm.StateNone || len(r.Data) != 0 {
			t.Fatalf("ожидалась пустая запись, получено %+v", r)
		}
	})

	t.Run("set then get", func(t *testing.T) {
		s := newStore(t)
		want := fsm.Record{State: "a", Data: []byte(`{"x":1}`)}
		if err := s.Set(ctx, 1, want); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := s.Get(ctx, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.State != want.State || !bytes.Equal(got.Data, want.Data) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("delete", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set(ctx, 1, fsm.Record{State: "a"}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Delete(ctx, 1); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		r, err := s.Get(ctx, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if r.State != fsm.StateNone {
			t.Fatalf("ожидался StateNone после Delete, получено %q", r.State)
		}
	})

	t.Run("cas from none on missing record", func(t *testing.T) {
		s := newStore(t)
		if err := s.CompareAndSet(ctx, 1, fsm.StateNone, fsm.Record{State: "a"}); err != nil {
			t.Fatalf("CompareAndSet: %v", err)
		}
		r, _ := s.Get(ctx, 1)
		if r.State != "a" {
			t.Fatalf("state = %q, want a", r.State)
		}
	})

	t.Run("cas mismatch keeps record", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set(ctx, 1, fsm.Record{State: "a", Data: []byte("1")}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		err := s.CompareAndSet(ctx, 1, "b", fsm.Record{State: "c"})
		if !errors.Is(err, fsm.ErrStateMismatch) {
			t.Fatalf("ожидался ErrStateMismatch, получено %v", err)
		}
		r, _ := s.Get(ctx, 1)
		if r.State != "a" || string(r.Data) != "1" {
			t.Fatalf("запись изменилась: %+v", r)
		}
	})

	t.Run("cas concurrent allows exactly one", func(t *testing.T) {
		s := newStore(t)
		if err := s.Set(ctx, 1, fsm.Record{State: "a"}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		var ok atomic.Int32
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				if err := s.CompareAndSet(ctx, 1, "a", fsm.Record{State: "b"}); err == nil {
					ok.Add(1)
				}
			})
		}
		wg.Wait()
		if ok.Load() != 1 {
			t.Fatalf("успешных CAS = %d, want 1", ok.Load())
		}
	})
}
