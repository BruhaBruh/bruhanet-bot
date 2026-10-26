package fsm

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func Middleware(store Storage) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			if id := tgupdate.UserID(u); id != 0 {
				ctx = WithContext(ctx, For(store, id))
			}
			next(ctx, b, u)
		}
	}
}
