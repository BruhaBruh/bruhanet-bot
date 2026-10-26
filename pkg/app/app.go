package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type fataler interface {
	Fatal() <-chan error
}

type App interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	StartTimeout() time.Duration
	StopTimeout() time.Duration
}

func Run(a App) error {
	startCtx, cancelStart := context.WithTimeout(context.Background(), a.StartTimeout())
	defer cancelStart()

	if err := a.Start(startCtx); err != nil {
		return fmt.Errorf("failed to start app: %w", err)
	}

	signalCtx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	var fatal <-chan error
	if withFatal, ok := a.(fataler); ok {
		fatal = withFatal.Fatal()
	}

	var runErr error

	select {
	case <-signalCtx.Done():
	case err := <-fatal:
		if err != nil {
			runErr = fmt.Errorf("component finished with error: %w", err)
		}
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), a.StopTimeout())
	defer cancelStop()

	if err := stopWithin(stopCtx, a); err != nil {
		return errors.Join(runErr, err)
	}

	return runErr
}

func stopWithin(ctx context.Context, a App) error {
	done := make(chan error, 1)

	go func() { done <- a.Stop(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to stop app: %w", err)
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("failed to stop app by timeout %s: %w", a.StopTimeout(), ctx.Err())
	}
}
