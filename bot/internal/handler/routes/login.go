package routes

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const loginTTL = 5 * time.Minute

func (h *handlers) login(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.isAdmin(u) {
		h.start(ctx, b, u)
		return
	}
	raw, hash, err := auth.NewSecret()
	if err == nil {
		err = h.pay.CreateLoginToken(ctx, hash, h.now().Add(loginTTL))
	}
	if err != nil {
		stlog.Error(ctx, "failed to create login token", "error", err)
		send(ctx, b, h.cfg.Telegram.AdminID, nil, errorText)
		return
	}
	link := strings.TrimRight(h.cfg.Admin.BaseURL, "/") + "/auth?token=" + raw
	send(ctx, b, h.cfg.Telegram.AdminID, keyboard.Inline(keyboard.Row(keyboard.URL("Открыть админку", link))),
		"Вход в админку: "+bot.EscapeMarkdown(link), "Ссылка одноразовая, действует 5 минут")
}
