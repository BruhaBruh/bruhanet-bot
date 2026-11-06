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
		t.Fatalf("lifecycle.Start expected to succeed")
	}
	if !isStarted {
		t.Fatalf("lifecycle expected to be enabled")
	}
	if err := l.Stop(t.Context()); err != nil {
		t.Fatalf("lifecycle.Stop expected to succeed")
	}
	if isStarted {
		t.Fatalf("lifecycle expected to be disabled")
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
		t.Fatalf("lifecycle.Start expected to fail")
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
		t.Fatalf("lifecycle.Start expected to succeed")
	}
	if err := l.Stop(t.Context()); err == nil {
		t.Fatalf("lifecycle.Stop expected to fail")
	}
}
