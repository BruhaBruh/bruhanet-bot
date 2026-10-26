package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
)

const (
	defaultStartTimeout = 15 * time.Second
	defaultStopTimeout  = 30 * time.Second
)

type component struct {
	name string
	lc   lifecycle.Lifecycle
	args []any
}

type CompositeApp struct {
	components   []component
	started      []component
	startTimeout time.Duration
	stopTimeout  time.Duration
	startedHook  func()
	stoppedHook  func()
	log          *slog.Logger
	fatal        <-chan error
}

func New(opts ...Option) *CompositeApp {
	composite := &CompositeApp{
		startTimeout: defaultStartTimeout,
		stopTimeout:  defaultStopTimeout,
		startedHook:  func() {},
		stoppedHook:  func() {},
		log:          slog.Default(),
	}

	for _, opt := range opts {
		opt(composite)
	}

	return composite
}

func (c *CompositeApp) Start(ctx context.Context) error {
	for _, comp := range c.components {
		c.log.Info("start component", comp.args...)

		if err := comp.lc.Start(ctx); err != nil {
			startErr := fmt.Errorf("failed to start component %s: %w", comp.name, err)

			if stopErr := c.Stop(ctx); stopErr != nil {
				return errors.Join(startErr, stopErr)
			}

			return startErr
		}

		c.started = append(c.started, comp)
	}

	c.startedHook()

	return nil
}

func (c *CompositeApp) Stop(ctx context.Context) error {
	var errs []error

	for _, comp := range slices.Backward(c.started) {
		c.log.Info("stop component", comp.args...)

		if err := comp.lc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop component %s: %w", comp.name, err))
		}
	}

	c.started = nil

	c.stoppedHook()

	return errors.Join(errs...)
}

func (c *CompositeApp) StartTimeout() time.Duration {
	return c.startTimeout
}

func (c *CompositeApp) StopTimeout() time.Duration {
	return c.stopTimeout
}

func (c *CompositeApp) Fatal() <-chan error { return c.fatal }
