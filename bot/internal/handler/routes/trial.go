package routes

import (
	"context"
	"errors"
	"strconv"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const (
	routeTrial    = "trial"
	routeTrialYes = "trialy"
)

// trialOffered — показывать ли кнопку «Пробный период» в меню: включён в настройках, срок в панели не действует и
// БД не против. Решает проверка в StartTrial; ошибка проверки кнопку прячет.
func (h *handlers) trialOffered(ctx context.Context, id int64, user *api.UserItemInfo) bool {
	set := h.c.Settings(ctx)
	if !set.TrialEnabled() || !payflow.TrialPanelOK(user, h.now()) {
		return false
	}
	ok, err := h.pay.TrialAllowed(ctx, id, set.AdvanceLimit)
	if err != nil {
		stlog.Warn(ctx, "failed to check trial", "error", err)
	}
	return ok
}

func (h *handlers) trialUnavailable(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, h.backKeyboard(ctx, routeMenu), h.c.Text(ctx, "trial.unavailable", nil))
}

func trialVars(days, devices int) map[string]string {
	return map[string]string{"days": strconv.Itoa(days), "devices": strconv.Itoa(devices)}
}

// trial — «Верно?» с днями и устройствами из настроек. Окончательно решает trialYes.
func (h *handlers) trial(ctx context.Context, b *bot.Bot, u *models.Update) {
	set := h.c.Settings(ctx)
	if !set.TrialEnabled() {
		h.start(ctx, b, u)
		return
	}
	ok, err := h.pay.TrialAllowed(ctx, tgupdate.UserID(u), set.AdvanceLimit)
	switch {
	case err != nil:
		stlog.Error(ctx, "failed to check trial", "error", err)
		h.showError(ctx, b, u)
	case !ok:
		h.trialUnavailable(ctx, b, u)
	default:
		show(ctx, b, u, h.kb(ctx, "trial_confirm", "trial.confirm").
			callback("button.yes", routeTrialYes, keyboard.StyleSuccess).
			callback("button.no", routeMenu, keyboard.StyleDanger).markup(),
			h.c.Text(ctx, "trial.confirm", trialVars(set.TrialDays, set.TrialDevices)))
	}
}

// trialYes — «Да»: выдача в транзакции хранилища вместе с панелью. Уведомление со сроком кладётся в outbox там же;
// на экране — меню (кнопки «Пробный» там уже нет).
func (h *handlers) trialYes(ctx context.Context, b *bot.Bot, u *models.Update) {
	set := h.c.Settings(ctx)
	if !set.TrialEnabled() {
		h.start(ctx, b, u)
		return
	}
	from := tgupdate.User(u)
	_, err := payflow.StartTrial(ctx, h.pay, h.rw, payflow.TrialRequest{
		TelegramID: from.ID, Username: from.Username, Days: set.TrialDays, Devices: set.TrialDevices,
		BaseDevices: set.DevicesBase, AdvanceLimit: set.AdvanceLimit, At: h.now(), Squad: h.cfg.Remnawave.InternalSquadId,
		NotFound: func(err error) bool { return errors.Is(err, remnawave.ErrUserNotFound) },
	})
	switch {
	case errors.Is(err, model.ErrTrialUnavailable), errors.Is(err, store.ErrBlocked):
		h.trialUnavailable(ctx, b, u)
	case err != nil:
		stlog.Error(ctx, "failed to start trial", "error", err)
		h.showError(ctx, b, u)
	default:
		h.start(ctx, b, u)
	}
}
