package lifecycle

import "context"

type Lifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

func New(
	start func(ctx context.Context) error,
	stop func(ctx context.Context) error,
) Lifecycle {
	return &funcs{start, stop}
}

type funcs struct {
	start func(ctx context.Context) error
	stop  func(ctx context.Context) error
}

func (f *funcs) Start(ctx context.Context) error {
	if f.start == nil {
		return nil
	}
	return f.start(ctx)
}

func (f *funcs) Stop(ctx context.Context) error {
	if f.stop == nil {
		return nil
	}
	return f.stop(ctx)
}
