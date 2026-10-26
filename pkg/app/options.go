package app

import (
	"log/slog"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
)

type Option func(*CompositeApp)

func WithComponent(name string, lc lifecycle.Lifecycle, args ...any) Option {
	return func(c *CompositeApp) {
		c.components = append(c.components, component{
			name: name,
			lc:   lc,
			args: append(args, "component", name),
		})
	}
}

func WithStartTimeout(timeout time.Duration) Option {
	return func(c *CompositeApp) { c.startTimeout = timeout }
}

func WithStopTimeout(timeout time.Duration) Option {
	return func(c *CompositeApp) { c.stopTimeout = timeout }
}

func WithStarted(hook func()) Option {
	return func(c *CompositeApp) { c.startedHook = hook }
}

func WithStopped(hook func()) Option {
	return func(c *CompositeApp) { c.stoppedHook = hook }
}

func WithLogger(log *slog.Logger) Option {
	return func(c *CompositeApp) { c.log = log }
}

func WithFatal(fatal <-chan error) Option {
	return func(c *CompositeApp) { c.fatal = fatal }
}
