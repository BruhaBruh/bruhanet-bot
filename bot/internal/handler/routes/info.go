package routes

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func (h *handlers) info(ctx context.Context, b *bot.Bot, u *models.Update) {
	telegramID := tgupdate.UserID(u)
	back := keyboard.Row(keyboard.Callback("Назад", routeMenu))
	user, err := h.rw.GetUser(ctx, telegramID)
	if errors.Is(err, remnawave.ErrUserNotFound) {
		show(ctx, b, u, keyboard.Inline(
			keyboard.Row(keyboard.Callback("Купить", routePay).Style(keyboard.StyleSuccess)),
			back,
		), "*Информация о подписке*", "", "*Статус*", "Не куплена")
		return
	}
	if err != nil {
		stlog.Error(ctx, "failed to get remnawave user", "error", err)
		h.showError(ctx, b, u)
		return
	}

	allowed := "∞"
	if user.TrafficLimitBytes > 0 {
		allowed = formatBytes(user.TrafficLimitBytes)
	}
	traffic := formatBytes(user.UserTraffic.UsedTrafficBytes) + " из " + allowed

	show(ctx, b, u, keyboard.Inline(
		keyboard.Row(
			keyboard.Callback("Продлить", routePay),
			keyboard.Copy("Подписка", user.SubscriptionUrl).Style(keyboard.StyleSuccess),
		),
		back,
	),
		"*Информация о подписке*",
		"",
		"*Статус*",
		bot.EscapeMarkdown(statusText(user.Status)),
		"",
		"*Трафик*",
		bot.EscapeMarkdown(traffic),
		"",
		"*Дата окончания подписки*",
		bot.EscapeMarkdown(formatTime(user.ExpireAt)),
		"",
		"*Ваш Telegram ID*",
		fmt.Sprintf("%d", telegramID),
	)
}

func statusText(s api.UserItemInfoStatus) string {
	switch s {
	case api.UserItemInfoStatusACTIVE:
		return "Активна"
	case api.UserItemInfoStatusDISABLED:
		return "Отключена"
	case api.UserItemInfoStatusLIMITED:
		return "Ограничена"
	case api.UserItemInfoStatusEXPIRED:
		return "Исчерпана"
	}
	return "Неизвестно"
}
