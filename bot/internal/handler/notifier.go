package handler

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/reminder"
)

var _ reminder.Notifier = (*Notifier)(nil)

// Notifier доставляет напоминания через Telegram.
type Notifier struct{ b *bot.Bot }

func NewNotifier(b *bot.Bot) *Notifier { return &Notifier{b: b} }

func (n *Notifier) Remind(ctx context.Context, telegramID int64, kind reminder.Kind, expireAt time.Time, subURL string) error {
	text, kb := routes.ReminderMessage(string(kind), expireAt, subURL)
	_, err := n.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      telegramID,
		Text:        text,
		ParseMode:   models.ParseModeMarkdown,
		ReplyMarkup: kb,
	})
	if errors.Is(err, bot.ErrorForbidden) || errors.Is(err, bot.ErrorBadRequest) {
		return errors.Join(reminder.ErrUndeliverable, err)
	}
	return err
}
