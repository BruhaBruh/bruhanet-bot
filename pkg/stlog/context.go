package stlog

import (
	"context"
	"log/slog"
)

type loggerCtxKeyType int8

const loggerCtxKey = loggerCtxKeyType(0)

func Wrap(ctx context.Context, logger *slog.Logger, args ...any) context.Context {
	newCtx := context.WithValue(ctx, loggerCtxKey, logger)
	if len(args) == 0 {
		return newCtx
	}
	return With(newCtx, args...)
}

func With(ctx context.Context, args ...any) context.Context {
	return Wrap(ctx, Logger(ctx).With(args...))
}

func Logger(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func L(ctx context.Context) *slog.Logger {
	return Logger(ctx)
}
