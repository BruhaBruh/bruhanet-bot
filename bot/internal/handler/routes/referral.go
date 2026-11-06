package routes

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const routeReferral = "referral"

// botUsername — username самого бота из getMe; кэшируется после первого успеха (он не меняется за жизнь процесса).
func (h *handlers) botUsername(ctx context.Context, b *bot.Bot) (string, error) {
	h.meMu.Lock()
	defer h.meMu.Unlock()
	if h.me != "" {
		return h.me, nil
	}
	me, err := b.GetMe(ctx)
	if err != nil {
		return "", err
	}
	h.me = me.Username
	return h.me, nil
}

// referral — экран «Пригласить друга»: личная ссылка t.me/<бот>?start=ref_<id> и кнопка копирования. Рефералка выключена
// (дни = 0) — обычное меню, ссылка не показывается.
func (h *handlers) referral(ctx context.Context, b *bot.Bot, u *models.Update) {
	set := h.c.Settings(ctx)
	if !set.ReferralEnabled() {
		h.start(ctx, b, u)
		return
	}
	name, err := h.botUsername(ctx, b)
	if err != nil || name == "" {
		stlog.Error(ctx, "failed to get bot username", "error", err)
		h.showError(ctx, b, u)
		return
	}
	link := "https://t.me/" + name + "?start=" + refPrefix + strconv.FormatInt(tgupdate.UserID(u), 10)
	show(ctx, b, u, keyboard.Inline(
		keyboard.Row(copyButton(ctx, h.c, "button.copy_link", link, keyboard.StyleSuccess)),
		keyboard.Row(callbackButton(ctx, h.c, "button.back", routeMenu, keyboard.StyleDefault)),
	), h.c.Text(ctx, "referral.menu", map[string]string{"days": strconv.Itoa(set.ReferralDays), "link": link}))
}
