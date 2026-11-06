package app_test

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
)

func TestRunStopsOnSignal(t *testing.T) {
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithStartTimeout(time.Second),
		app.WithStopTimeout(time.Second),
		app.WithComponent("компонент", rec.component("компонент", nil, nil)),
	)

	go func() {
		time.Sleep(50 * time.Millisecond)

		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			t.Errorf("failed to send SIGTERM: %v", err)
		}
	}()

	if err := app.Run(a); err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}

	want := []string{"start:компонент", "stop:компонент"}
	if !equal(rec.events, want) {
		t.Fatalf("sequence %v, want %v", rec.events, want)
	}
}

func TestRunReturnsStartError(t *testing.T) {
	boom := errors.New("failed to start")
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithStartTimeout(time.Second),
		app.WithStopTimeout(time.Second),
		app.WithComponent("компонент", rec.component("компонент", boom, nil)),
	)

	if err := app.Run(a); !errors.Is(err, boom) {
		t.Fatalf("Run() returned %v, want a wrapper of boom", err)
	}
}

func TestRunStopsOnFatalError(t *testing.T) {
	boom := errors.New("server crashed")
	fatal := make(chan error, 1)
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithStartTimeout(time.Second),
		app.WithStopTimeout(time.Second),
		app.WithFatal(fatal),
		app.WithComponent("компонент", rec.component("компонент", nil, nil)),
	)

	fatal <- boom

	err := app.Run(a)
	if !errors.Is(err, boom) {
		t.Fatalf("Run() returned %v, want a wrapper of boom", err)
	}

	want := []string{"start:компонент", "stop:компонент"}
	if !equal(rec.events, want) {
		t.Fatalf("sequence %v, want %v", rec.events, want)
	}
}

// TestRunEnforcesStopTimeout — доказательство, что предел времени остановки
// настоящий: компонент намеренно игнорирует контекст, и Run обязан вернуть
// управление, а не ждать его вечно.
func TestRunEnforcesStopTimeout(t *testing.T) {
	fatal := make(chan error, 1)

	stuck := lifecycle.New(
		func(context.Context) error { return nil },
		func(context.Context) error {
			time.Sleep(5 * time.Second)

			return nil
		},
	)

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithStartTimeout(time.Second),
		app.WithStopTimeout(100*time.Millisecond),
		app.WithFatal(fatal),
		app.WithComponent("зависший", stuck),
	)

	fatal <- errors.New("time to shut down")

	done := make(chan error, 1)

	go func() { done <- app.Run(a) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run() returned %v, want DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return: stop timeout does not work")
	}
}
