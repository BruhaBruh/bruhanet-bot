package middleware

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// AutoAnswer отвечает на callback после хендлера, если тот не ответил сам.
// Глобально — без текста; на роуте — с toast из message (строки через "\n").
// При панике хендлера ответ роута не уходит: отвечает глобальный, пустым.
func AutoAnswer(message ...string) bot.Middleware {
	text := strings.Join(message, "\n")
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			next(ctx, b, u)
			if u.CallbackQuery == nil || UpdateInfoFrom(ctx).Answered() {
				return
			}
			Answer(ctx, b, u, text)
		}
	}
}

func Answer(ctx context.Context, b *bot.Bot, u *models.Update, text string) {
	if u.CallbackQuery == nil {
		return
	}
	info := UpdateInfoFrom(ctx)
	if info.Answered() {
		return
	}
	info.MarkAnswered()
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: u.CallbackQuery.ID,
		Text:            text,
	})
	if err != nil {
		stlog.Warn(ctx, "failed to answer callback query", "error", err)
	}
}
