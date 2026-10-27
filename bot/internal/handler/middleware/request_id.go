package middleware

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func RequestID(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		id, err := uuid.NewV7()
		if err != nil {
			id = uuid.New()
		}
		ctx = observability.WithRequestID(ctx, id.String())
		ctx = withInfo(ctx, &UpdateInfo{})
		next(stlog.With(ctx, "request_id", id.String()), b, u)
	}
}

func RequestIDFrom(ctx context.Context) string {
	return observability.RequestID(ctx)
}
