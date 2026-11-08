package routes

import (
	"context"
	"errors"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// condEnv считает условия показа кнопки (каталог texts.Conditions) при построении клавиатуры. Дорогое (юзер панели,
// пробный период) считается лениво и один раз; не заданное экраном — ложь.
type condEnv struct {
	ctx   context.Context
	c     *content.Provider
	panel func() bool // юзер есть в панели
	trial func() bool // пробный доступен
	memo  map[string]bool
}

func (e *condEnv) eval(id string) bool {
	if v, ok := e.memo[id]; ok {
		return v
	}
	var v bool
	switch id {
	case "in_panel":
		v = e.panel != nil && e.panel()
	case "trial_available":
		v = e.trial != nil && e.trial()
	case "devices_sale_on":
		v = e.c.Settings(e.ctx).DevicesEnabled()
	case "gifts_on":
		v = giftOffered(e.ctx, e.c)
	case "referral_on":
		v = e.c.Settings(e.ctx).ReferralEnabled()
	default:
		stlog.Error(e.ctx, "unknown keyboard condition", "condition", id)
	}
	if e.memo == nil {
		e.memo = map[string]bool{}
	}
	e.memo[id] = v
	return v
}

// known сообщает, что юзер панели уже прочитан: user — он сам, nil — его там нет.
func (b *kbBuilder) known(telegramID int64, user *api.UserItemInfo, trial trialOffer) *kbBuilder {
	b.env.panel = func() bool { return user != nil }
	if trial != nil {
		b.env.trial = func() bool { return trial(b.ctx, telegramID, user) }
	}
	return b
}

// inPanel — юзер в панели заведомо (или заведомо нет), например напоминание об окончании подписки.
func (b *kbBuilder) inPanel(v bool) *kbBuilder {
	b.env.panel = func() bool { return v }
	return b
}

// viaPanel читает юзера панели (получателя из контекста) лениво, только если условие этого просит.
func (b *kbBuilder) viaPanel(rw Remnawave, trial trialOffer) *kbBuilder {
	id := content.RecipientID(b.ctx)
	var (
		user *api.UserItemInfo
		err  error
		done bool
	)
	load := func() {
		if !done {
			user, err = rw.GetUser(b.ctx, id)
			done = true
		}
	}
	b.env.panel = func() bool { load(); return err == nil }
	b.env.trial = func() bool {
		load()
		switch {
		case trial == nil:
			return false
		case err == nil:
			return trial(b.ctx, id, user)
		case errors.Is(err, remnawave.ErrUserNotFound):
			return trial(b.ctx, id, nil)
		}
		return false
	}
	return b
}
