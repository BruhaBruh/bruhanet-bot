package routes

import (
	"context"
	"errors"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
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
	// Ссылка со страницы входа админки (`?start=login`): админу — кнопка входа, как у /login.
	// Остальным — обычное приветствие, ничего не выдаём (и не отличаем от простого /start).
	if h.isAdmin(u) && isStartLogin(u) {
		h.login(ctx, b, u)
		return
	}
	h.leavePromo(ctx, tgupdate.UserID(u))
	show(ctx, b, u, h.mainKeyboard(ctx, tgupdate.UserID(u)), h.c.Text(ctx, "menu.greeting", nil))
}

// isStartLogin — сообщение ровно «/start login» (deep link payload).
func isStartLogin(u *models.Update) bool {
	if u.Message == nil {
		return false
	}
	f := strings.Fields(u.Message.Text)
	return len(f) == 2 && (f[0] == "/start" || strings.HasPrefix(f[0], "/start@")) && f[1] == "login"
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
	return mainKeyboard(ctx, h.c, h.rw, telegramID)
}

func mainKeyboard(ctx context.Context, c *content.Provider, rw Remnawave, telegramID int64) *models.InlineKeyboardMarkup {
	support := urlButton(ctx, c, "button.support", c.Settings(ctx).SupportURL, keyboard.StyleDanger)
	promoBtn := callbackButton(ctx, c, "button.promo", routePromo, keyboard.StyleDefault)
	user, err := rw.GetUser(ctx, telegramID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return keyboard.Inline(
			keyboard.Row(callbackButton(ctx, c, "button.buy", routePay, keyboard.StyleSuccess)),
			keyboard.Row(promoBtn, support),
		)
	case err != nil:
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		return keyboard.Inline(keyboard.Row(callbackButton(ctx, c, "button.service_down", routeMenu, keyboard.StyleDefault)))
	}
	return keyboard.Inline(
		keyboard.Row(
			callbackButton(ctx, c, "button.info", routeInfo, keyboard.StylePrimary),
			callbackButton(ctx, c, "button.pay", routePay, keyboard.StyleDefault),
		),
		keyboard.Row(
			copyButton(ctx, c, "button.subscription", user.SubscriptionUrl, keyboard.StyleSuccess),
			support,
		),
		keyboard.Row(promoRow(ctx, c, promoBtn)...),
	)
}

// promoRow — «Промокод», а при включённой продаже устройств рядом «Устройства».
func promoRow(ctx context.Context, c *content.Provider, promoBtn keyboard.Button) []keyboard.Button {
	if !c.Settings(ctx).DevicesEnabled() {
		return []keyboard.Button{promoBtn}
	}
	return []keyboard.Button{promoBtn, callbackButton(ctx, c, "button.devices", routeDevices, keyboard.StyleDefault)}
}
