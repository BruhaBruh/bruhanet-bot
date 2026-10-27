package stlog

import "context"

func Debug(ctx context.Context, msg string, args ...any) {
	L(ctx).DebugContext(ctx, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	L(ctx).InfoContext(ctx, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	L(ctx).WarnContext(ctx, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	L(ctx).ErrorContext(ctx, msg, args...)
}
