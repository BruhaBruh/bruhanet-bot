package middleware

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func AdminOnly(adminID int64) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			if tgupdate.UserID(u) != adminID {
				stlog.Warn(ctx, "admin route denied")
				Answer(ctx, b, u, "Недостаточно прав")
				return
			}
			next(ctx, b, u)
		}
	}
}
