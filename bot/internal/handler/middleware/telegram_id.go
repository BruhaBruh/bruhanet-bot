package middleware

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func TelegramID(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		args := []any{
			"telegram_id", tgupdate.UserID(u),
			"chat_id", tgupdate.ChatID(u),
			"update_type", tgupdate.Type(u),
		}
		if u.CallbackQuery != nil {
			args = append(args, "callback_data", u.CallbackQuery.Data)
		}
		next(stlog.With(ctx, args...), b, u)
	}
}
