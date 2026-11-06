package middleware

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

// TextUser — автор апдейта как получатель текстов: {{.name}} и др. работают без правок в хендлерах.
func TextUser(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		if user := tgupdate.User(u); user != nil {
			ctx = content.WithUser(ctx, user.ID, user.FirstName, user.Username)
		}
		next(ctx, b, u)
	}
}
