package routes

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const loginTTL = 5 * time.Minute

func (h *handlers) isAdmin(u *models.Update) bool {
	return tgupdate.UserID(u) == h.cfg.Telegram.AdminID
}

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
		send(ctx, b, h.cfg.Telegram.AdminID, nil, h.c.Text(ctx, "error.generic", nil))
		return
	}
	link := strings.TrimRight(h.cfg.Admin.BaseURL, "/") + "/auth?token=" + raw
	// Ссылка только в кнопке и без превью: краулер превью Telegram сам открыл бы /auth и сжёг одноразовый токен.
	text := h.c.Text(ctx, "login.message", nil)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:             h.cfg.Telegram.AdminID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		ReplyMarkup:        h.kb(ctx, "login", "login.message").url("button.login", link, keyboard.StylePrimary).markup(),
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	})
	if errors.Is(err, bot.ErrorBadRequest) {
		// Telegram не принимает, например, localhost в URL-кнопке (dev) и не делает такую ссылку кликабельной:
		// шлём её моноширинным блоком (копируется тапом), тоже без превью.
		stlog.Warn(ctx, "login button rejected, sending plain link", "error", err)
		fb := &bot.SendMessageParams{
			ChatID:             h.cfg.Telegram.AdminID,
			Text:               h.c.Text(ctx, "login.fallback", map[string]string{"link": link}),
			ParseMode:          models.ParseModeHTML,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		}
		if kb := h.kb(ctx, "", "login.fallback").markup(); kb != nil { // свои кнопки админа; без них клавиатуры нет
			fb.ReplyMarkup = kb
		}
		_, err = b.SendMessage(ctx, fb)
	}
	if err != nil {
		stlog.Warn(ctx, "failed to send login link", "error", err)
		send(ctx, b, h.cfg.Telegram.AdminID, nil, h.c.Text(ctx, "error.generic", nil))
	}
}
