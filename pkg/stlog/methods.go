package stlog

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

func Debug(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelDebug, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelInfo, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelWarn, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelError, msg, args...)
}

// log пропускает свои кадры, чтобы AddSource указывал на вызывающий код.
func log(ctx context.Context, level slog.Level, msg string, args ...any) {
	l := L(ctx)
	if !l.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // runtime.Callers, log, Debug/Info/...
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = l.Handler().Handle(ctx, r)
}
