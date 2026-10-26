package routes

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func (h *handlers) start(ctx context.Context, b *bot.Bot, u *models.Update) {
	if t := tgupdate.Type(u); t == tgupdate.TypeEditedMessage || t == tgupdate.TypeOther || !isPrivate(u) {
		return
	}
	if tgupdate.ChatID(u) == 0 {
		return
	}
	show(ctx, b, u, h.mainKeyboard(ctx, tgupdate.UserID(u)), "Здравствуйте, *"+bot.EscapeMarkdown(displayName(u))+"*")
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

func (h *handlers) mainKeyboard(ctx context.Context, telegramID int64) *models.InlineKeyboardMarkup {
	support := keyboard.URL("Поддержка", h.cfg.Support.URL).Style(keyboard.StyleDanger)
	user, err := h.rw.GetUser(ctx, telegramID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return keyboard.Inline(
			keyboard.Row(keyboard.Callback("Купить", routePay).Style(keyboard.StyleSuccess)),
			keyboard.Row(support),
		)
	case err != nil:
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		return keyboard.Inline(keyboard.Row(keyboard.Callback("Сервис не работает", routeMenu)))
	}
	return keyboard.Inline(
		keyboard.Row(
			keyboard.Callback("Информация", routeInfo).Style(keyboard.StylePrimary),
			keyboard.Callback("Оплатить", routePay),
		),
		keyboard.Row(
			keyboard.Copy("Подписка", user.SubscriptionUrl).Style(keyboard.StyleSuccess),
			support,
		),
	)
}
