package fsm

import "context"

type ctxKey struct{}

func WithContext(ctx context.Context, f *FSM) context.Context {
	return context.WithValue(ctx, ctxKey{}, f)
}

func From(ctx context.Context) *FSM {
	f, _ := ctx.Value(ctxKey{}).(*FSM)
	return f
}
