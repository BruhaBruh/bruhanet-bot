package fsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

type payment struct {
	Month int `json:"month"`
}

func TestPutAndData(t *testing.T) {
	ctx := context.Background()
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := fsm.Put(ctx, f, "await", payment{Month: 6}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	st, err := f.State(ctx)
	if err != nil || st != "await" {
		t.Fatalf("State = %q, %v", st, err)
	}
	p, err := fsm.Data[payment](ctx, f)
	if err != nil || p.Month != 6 {
		t.Fatalf("Data = %+v, %v", p, err)
	}
}

func TestDataMissing(t *testing.T) {
	f := fsm.For(fsm.NewInMemory(), 1)
	if _, err := fsm.Data[payment](context.Background(), f); !errors.Is(err, fsm.ErrNoData) {
		t.Fatalf("ожидался ErrNoData, получено %v", err)
	}
}

func TestDataDecodeError(t *testing.T) {
	ctx := context.Background()
	s := fsm.NewInMemory()
	if err := s.Set(ctx, 1, fsm.Record{State: "a", Data: []byte("not-json")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, err := fsm.Data[payment](ctx, fsm.For(s, 1)); !errors.Is(err, fsm.ErrDecode) {
		t.Fatalf("ожидался ErrDecode, получено %v", err)
	}
}

func TestSetKeepsData(t *testing.T) {
	ctx := context.Background()
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := fsm.Put(ctx, f, "a", payment{Month: 1}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := f.Set(ctx, "b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	p, err := fsm.Data[payment](ctx, f)
	if err != nil || p.Month != 1 {
		t.Fatalf("data потеряна: %+v, %v", p, err)
	}
	if st, _ := f.State(ctx); st != "b" {
		t.Fatalf("State = %q, want b", st)
	}
}

func TestTransition(t *testing.T) {
	ctx := context.Background()
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := fsm.Put(ctx, f, "review", payment{Month: 3}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := fsm.Transition(ctx, f, "review", fsm.StateNone, nil); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if _, err := fsm.Data[payment](ctx, f); !errors.Is(err, fsm.ErrNoData) {
		t.Fatalf("после Transition с nil data ожидался ErrNoData, получено %v", err)
	}
	if err := fsm.Transition(ctx, f, "review", fsm.StateNone, nil); !errors.Is(err, fsm.ErrStateMismatch) {
		t.Fatalf("повторный Transition: ожидался ErrStateMismatch, получено %v", err)
	}
}

func TestClear(t *testing.T) {
	ctx := context.Background()
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := f.Set(ctx, "a"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := f.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if st, _ := f.State(ctx); st != fsm.StateNone {
		t.Fatalf("State = %q после Clear", st)
	}
}

func TestNilFSM(t *testing.T) {
	ctx := context.Background()
	var f *fsm.FSM
	if _, err := f.State(ctx); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("State: %v", err)
	}
	if err := f.Set(ctx, "a"); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("Set: %v", err)
	}
	if err := f.Clear(ctx); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("Clear: %v", err)
	}
	if err := fsm.Put(ctx, f, "a", nil); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("Put: %v", err)
	}
	if _, err := fsm.Data[payment](ctx, f); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("Data: %v", err)
	}
	if err := fsm.Transition(ctx, f, "a", "b", nil); !errors.Is(err, fsm.ErrNoFSM) {
		t.Errorf("Transition: %v", err)
	}
	if f.UserID() != 0 {
		t.Errorf("UserID = %d", f.UserID())
	}
}

func TestPutEncodeError(t *testing.T) {
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := fsm.Put(context.Background(), f, "a", make(chan int)); !errors.Is(err, fsm.ErrEncode) {
		t.Fatalf("ожидался ErrEncode, получено %v", err)
	}
}

func TestContext(t *testing.T) {
	ctx := context.Background()
	if fsm.From(ctx) != nil {
		t.Fatal("From без FSM должен вернуть nil")
	}
	f := fsm.For(fsm.NewInMemory(), 7)
	if got := fsm.From(fsm.WithContext(ctx, f)); got != f {
		t.Fatal("From вернул не тот FSM")
	}
}

func TestPutTypedNilIsNoData(t *testing.T) {
	ctx := context.Background()
	f := fsm.For(fsm.NewInMemory(), 1)
	if err := fsm.Put(ctx, f, "a", (*payment)(nil)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := fsm.Data[payment](ctx, f); !errors.Is(err, fsm.ErrNoData) {
		t.Fatalf("Data: %v", err)
	}
}
