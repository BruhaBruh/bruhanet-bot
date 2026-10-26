package routes

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func (h *handlers) start(ctx context.Context, b *bot.Bot, u *models.Update) {
	if t := tgupdate.Type(u); t == tgupdate.TypeEditedMessage || t == tgupdate.TypeOther || !isPrivate(u) {
		return
	}
	chatID := tgupdate.ChatID(u)
	if chatID == 0 {
		return
	}
	send(ctx, b, chatID, h.mainKeyboard(), "Здравствуйте, *"+bot.EscapeMarkdown(displayName(u))+"*")
}

// isPrivate: недоступное сообщение колбэка считаем приватным, ответ всё равно уйдёт по ChatID.
func isPrivate(u *models.Update) bool {
	switch {
	case u.Message != nil:
		return u.Message.Chat.Type == models.ChatTypePrivate
	case u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil:
		return u.CallbackQuery.Message.Message.Chat.Type == models.ChatTypePrivate
	}
	return true
}

func (h *handlers) mainKeyboard() *models.InlineKeyboardMarkup {
	return keyboard.Inline(
		keyboard.Row(
			keyboard.Callback("Информация", routeInfo).Style(keyboard.StylePrimary),
			keyboard.Callback("Оплатить", routePay),
		),
		keyboard.Row(keyboard.URL("Поддержка", h.cfg.Support.URL).Style(keyboard.StyleDanger)),
	)
}
