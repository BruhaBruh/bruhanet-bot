package stlog

import "context"

func Debug(ctx context.Context, msg string, args ...any) {
	L(ctx).Debug(msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	L(ctx).Info(msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	L(ctx).Warn(msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	L(ctx).Error(msg, args...)
}
