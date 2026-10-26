package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
)

func TestStartAndStop(t *testing.T) {
	isStarted := false
	l := lifecycle.New(
		func(ctx context.Context) error {
			isStarted = true
			return nil
		},
		func(ctx context.Context) error {
			isStarted = false
			return nil
		},
	)
	if err := l.Start(t.Context()); err != nil {
		t.Fatalf("ожидалось что lifecycle.Start не вернет ошибку")
	}
	if !isStarted {
		t.Fatalf("ожидалось что lifecycle включен")
	}
	if err := l.Stop(t.Context()); err != nil {
		t.Fatalf("ожидалось что lifecycle.Stop не вернет ошибку")
	}
	if isStarted {
		t.Fatalf("ожидалось что lifecycle выключен")
	}
}

func TestStartError(t *testing.T) {
	l := lifecycle.New(
		func(ctx context.Context) error {
			return errors.New("failed")
		},
		func(ctx context.Context) error {
			return nil
		},
	)
	if err := l.Start(t.Context()); err == nil {
		t.Fatalf("ожидалось что lifecycle.Start вернет ошибку")
	}
}

func TestStopError(t *testing.T) {
	l := lifecycle.New(
		func(ctx context.Context) error {
			return nil
		},
		func(ctx context.Context) error {
			return errors.New("failed")
		},
	)
	if err := l.Start(t.Context()); err != nil {
		t.Fatalf("ожидалось что lifecycle.Start не вернет ошибку")
	}
	if err := l.Stop(t.Context()); err == nil {
		t.Fatalf("ожидалось что lifecycle.Stop вернет ошибку")
	}
}
