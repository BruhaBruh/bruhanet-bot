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
	h.leavePromo(ctx, tgupdate.UserID(u))
	show(ctx, b, u, h.mainKeyboard(ctx, tgupdate.UserID(u)), h.c.Text(ctx, "menu.greeting", nil))
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
	support := urlButton(ctx, h.c, "button.support", h.c.Settings(ctx).SupportURL, keyboard.StyleDanger)
	promoBtn := callbackButton(ctx, h.c, "button.promo", routePromo, keyboard.StyleDefault)
	user, err := h.rw.GetUser(ctx, telegramID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return keyboard.Inline(
			keyboard.Row(callbackButton(ctx, h.c, "button.buy", routePay, keyboard.StyleSuccess)),
			keyboard.Row(promoBtn, support),
		)
	case err != nil:
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		return keyboard.Inline(keyboard.Row(callbackButton(ctx, h.c, "button.service_down", routeMenu, keyboard.StyleDefault)))
	}
	return keyboard.Inline(
		keyboard.Row(
			callbackButton(ctx, h.c, "button.info", routeInfo, keyboard.StylePrimary),
			callbackButton(ctx, h.c, "button.pay", routePay, keyboard.StyleDefault),
		),
		keyboard.Row(
			copyButton(ctx, h.c, "button.subscription", user.SubscriptionUrl, keyboard.StyleSuccess),
			support,
		),
		keyboard.Row(promoBtn),
	)
}
