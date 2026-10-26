package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func WithLogger(logger *slog.Logger) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			next(stlog.Wrap(ctx, logger), b, u)
		}
	}
}

func Logger(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		start := time.Now()
		next(ctx, b, u)

		info := UpdateInfoFrom(ctx)
		if info.Panicked() {
			// паника уже залогирована Recovery со стеком
			return
		}
		stlog.Info(ctx, "update handled",
			"route", info.Route(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
