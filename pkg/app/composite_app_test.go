package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type recorder struct {
	events []string
}

func (r *recorder) component(name string, startErr, stopErr error) lifecycle.Lifecycle {
	return lifecycle.New(
		func(context.Context) error {
			r.events = append(r.events, "start:"+name)

			return startErr
		},
		func(context.Context) error {
			r.events = append(r.events, "stop:"+name)

			return stopErr
		},
	)
}

func TestStartRunsComponentsInOrder(t *testing.T) {
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithComponent("первый", rec.component("первый", nil, nil)),
		app.WithComponent("второй", rec.component("второй", nil, nil)),
	)

	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start() вернул ошибку: %v", err)
	}

	want := []string{"start:первый", "start:второй"}
	if !equal(rec.events, want) {
		t.Fatalf("порядок запуска %v, ожидался %v", rec.events, want)
	}
}

func TestStopRunsComponentsInReverseOrder(t *testing.T) {
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithComponent("первый", rec.component("первый", nil, nil)),
		app.WithComponent("второй", rec.component("второй", nil, nil)),
	)

	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start() вернул ошибку: %v", err)
	}

	rec.events = nil

	if err := a.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() вернул ошибку: %v", err)
	}

	want := []string{"stop:второй", "stop:первый"}
	if !equal(rec.events, want) {
		t.Fatalf("порядок остановки %v, ожидался %v", rec.events, want)
	}
}

func TestStartRollsBackStartedComponentsOnFailure(t *testing.T) {
	rec := &recorder{}
	boom := errors.New("не поднялся")

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithComponent("первый", rec.component("первый", nil, nil)),
		app.WithComponent("второй", rec.component("второй", boom, nil)),
		app.WithComponent("третий", rec.component("третий", nil, nil)),
	)

	err := a.Start(t.Context())
	if !errors.Is(err, boom) {
		t.Fatalf("Start() вернул %v, ожидалась обёртка над boom", err)
	}

	want := []string{"start:первый", "start:второй", "stop:первый"}
	if !equal(rec.events, want) {
		t.Fatalf("последовательность %v, ожидалась %v", rec.events, want)
	}
}

func TestStopJoinsErrors(t *testing.T) {
	rec := &recorder{}
	first := errors.New("первый не закрылся")
	second := errors.New("второй не закрылся")

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithComponent("первый", rec.component("первый", nil, first)),
		app.WithComponent("второй", rec.component("второй", nil, second)),
	)

	if err := a.Start(t.Context()); err != nil {
		t.Fatalf("Start() вернул ошибку: %v", err)
	}

	err := a.Stop(t.Context())

	if !errors.Is(err, first) {
		t.Fatalf("Stop() потерял ошибку первого компонента: %v", err)
	}

	if !errors.Is(err, second) {
		t.Fatalf("Stop() потерял ошибку второго компонента: %v", err)
	}
}

func TestStopSkipsComponentsThatNeverStarted(t *testing.T) {
	rec := &recorder{}

	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithComponent("первый", rec.component("первый", nil, nil)),
	)

	if err := a.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() без Start вернул ошибку: %v", err)
	}

	if len(rec.events) != 0 {
		t.Fatalf("Stop() без Start тронул компоненты: %v", rec.events)
	}
}

func TestTimeoutsHaveDefaults(t *testing.T) {
	a := app.New(app.WithLogger(quietLogger()))

	if a.StartTimeout() != 15*time.Second {
		t.Fatalf("умолчание StartTimeout = %s, ожидалось 15s", a.StartTimeout())
	}

	if a.StopTimeout() != 30*time.Second {
		t.Fatalf("умолчание StopTimeout = %s, ожидалось 30s", a.StopTimeout())
	}
}

func TestTimeoutOptionsOverrideDefaults(t *testing.T) {
	a := app.New(
		app.WithLogger(quietLogger()),
		app.WithStartTimeout(time.Second),
		app.WithStopTimeout(2*time.Second),
	)

	if a.StartTimeout() != time.Second {
		t.Fatalf("StartTimeout = %s, ожидалась 1s", a.StartTimeout())
	}

	if a.StopTimeout() != 2*time.Second {
		t.Fatalf("StopTimeout = %s, ожидались 2s", a.StopTimeout())
	}
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}

	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}
