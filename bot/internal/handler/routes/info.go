package routes

import (
	"context"
	"errors"

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
	back := keyboard.Row(keyboard.Callback(h.c.Button(ctx, "button.back"), routeMenu))
	user, err := h.rw.GetUser(ctx, telegramID)
	if errors.Is(err, remnawave.ErrUserNotFound) {
		show(ctx, b, u, keyboard.Inline(
			keyboard.Row(keyboard.Callback(h.c.Button(ctx, "button.buy"), routePay).Style(keyboard.StyleSuccess)),
			back,
		), h.c.Text(ctx, "menu.info_not_bought", nil))
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
			keyboard.Callback(h.c.Button(ctx, "button.renew"), routePay),
			keyboard.Copy(h.c.Button(ctx, "button.subscription"), user.SubscriptionUrl).Style(keyboard.StyleSuccess),
		),
		back,
	), h.c.Text(ctx, "menu.info", map[string]string{
		"status":   h.c.Text(ctx, statusKey(user.Status), nil),
		"traffic":  traffic,
		"expireAt": formatTime(user.ExpireAt),
	}))
}

func statusKey(s api.UserItemInfoStatus) string {
	switch s {
	case api.UserItemInfoStatusACTIVE:
		return "menu.status_active"
	case api.UserItemInfoStatusDISABLED:
		return "menu.status_disabled"
	case api.UserItemInfoStatusLIMITED:
		return "menu.status_limited"
	case api.UserItemInfoStatusEXPIRED:
		return "menu.status_expired"
	}
	return "menu.status_unknown"
}
