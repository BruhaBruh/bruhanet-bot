package routes

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/devices"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const routeDevices = "devices"

var (
	devPickRoute  = callback.New[int]("dev")  // dev:<сколько> — экран подтверждения
	devYesRoute   = callback.New[int]("devy") // devy:<сколько> — создать платёж
	buyNoDevRoute = callback.New[int]("buyd") // buyd:<месяцы> — продление без доп. устройств
)

// deviceChoices — сколько устройств можно докупить за раз.
var deviceChoices = []int{1, 2, 3, 5}

func pluralDevices(n int) string {
	return plural(n, "устройство", "устройства", "устройств")
}

func devicesCount(n int) string { return fmt.Sprintf("%d %s", n, pluralDevices(n)) }

// quote — цена тарифа с доп. устройствами юзера (или без них).
type quote struct {
	Month, Tariff, Extra, Addon, Total int
}

// quote считает цену тарифа: тариф + доп. устройства × цена × месяцы. keepExtra=false — «Без доп. устройств».
// Цена фиксируется при создании платежа вместе с Extra; подтверждение применяет сохранённое число, а не пересчитывает.
func (h *handlers) quote(ctx context.Context, userID int64, month int, keepExtra bool) (quote, error) {
	q := quote{Month: month, Tariff: h.c.Settings(ctx).Prices[month]}
	if keepExtra {
		extra, err := h.pay.ExtraDevices(ctx, userID)
		if err != nil {
			return q, err
		}
		q.Extra = extra
	}
	q.Addon = devices.TariffAddon(h.c.Settings(ctx).DevicePrice, q.Extra, month)
	q.Total = q.Tariff + q.Addon
	return q, nil
}

// hasPaidExtra — в цену тарифа входят платные доп. устройства (есть что показывать и сбрасывать).
func (q quote) hasPaidExtra() bool { return q.Extra > 0 && q.Addon > 0 }

func (q quote) vars(devicePrice int) map[string]string {
	v := tariffVars(q.Month, q.Total)
	v["tariff"] = strconv.Itoa(q.Tariff)
	v["devices"] = strconv.Itoa(q.Extra)
	v["devicePrice"] = strconv.Itoa(devicePrice)
	v["monthsCount"] = strconv.Itoa(q.Month)
	v["addon"] = strconv.Itoa(q.Addon)
	return v
}

func deviceVars(set settings.Settings, extra int) map[string]string {
	return map[string]string{
		"limit": strconv.Itoa(set.DevicesBase + extra), "base": strconv.Itoa(set.DevicesBase),
		"extra": strconv.Itoa(extra), "devicePrice": strconv.Itoa(set.DevicePrice),
	}
}

// devices — экран «Устройства». Продажа выключена или юзера нет в панели — обычное меню.
func (h *handlers) devices(ctx context.Context, b *bot.Bot, u *models.Update) {
	set := h.c.Settings(ctx)
	userID := tgupdate.UserID(u)
	user, extra, ok := h.deviceUser(ctx, b, u)
	if !ok {
		return
	}
	vars := deviceVars(set, extra)
	if devices.RemainingDays(user.ExpireAt, h.now()) == 0 {
		show(ctx, b, u, keyboard.Inline(
			keyboard.Row(callbackButton(ctx, h.c, "button.renew", routePay, keyboard.StyleSuccess)),
			keyboard.Row(callbackButton(ctx, h.c, "button.back", routeMenu, keyboard.StyleDefault)),
		), h.c.Text(ctx, "devices.no_term", vars))
		return
	}
	buttons := make([]keyboard.Button, 0, len(deviceChoices))
	for _, n := range deviceChoices {
		price, err := devices.ProratedPrice(set.DevicePrice, n, user.ExpireAt, h.now())
		if err != nil {
			break
		}
		buttons = append(buttons, keyboard.Callback(h.c.Text(ctx, "button.devices_add",
			map[string]string{"count": strconv.Itoa(n), "price": strconv.Itoa(price)}), devPickRoute.Data(n)).
			Style(h.c.Style(ctx, "button.devices_add", keyboard.StyleDefault)))
	}
	rows := keyboard.Grid(2, buttons...)
	rows = append(rows, keyboard.Row(callbackButton(ctx, h.c, "button.back", routeMenu, keyboard.StyleDefault)))
	h.leavePromo(ctx, userID)
	show(ctx, b, u, keyboard.Inline(rows...), h.c.Text(ctx, "devices.menu", vars))
}

// deviceUser — юзер панели и его доп. устройства; false — экран уже показан (меню или ошибка).
func (h *handlers) deviceUser(ctx context.Context, b *bot.Bot, u *models.Update) (*api.UserItemInfo, int, bool) {
	if !h.c.Settings(ctx).DevicesEnabled() {
		h.start(ctx, b, u)
		return nil, 0, false
	}
	userID := tgupdate.UserID(u)
	user, err := h.rw.GetUser(ctx, userID)
	if errors.Is(err, remnawave.ErrUserNotFound) {
		h.start(ctx, b, u)
		return nil, 0, false
	}
	var extra int
	if err == nil {
		extra, err = h.pay.ExtraDevices(ctx, userID)
	}
	if err != nil {
		stlog.Error(ctx, "failed to load devices", "error", err)
		h.showError(ctx, b, u)
		return nil, 0, false
	}
	return user, extra, true
}

// devicePrice — цена n устройств до конца подписки юзера; callback подделывается, поэтому n проверяется.
func (h *handlers) devicePrice(ctx context.Context, b *bot.Bot, u *models.Update, route callback.Route[int]) (n, price int, ok bool) {
	n, err := route.Parse(u.CallbackQuery.Data)
	if err != nil || !slices.Contains(deviceChoices, n) {
		h.devices(ctx, b, u)
		return 0, 0, false
	}
	user, _, ok := h.deviceUser(ctx, b, u)
	if !ok {
		return 0, 0, false
	}
	price, err = devices.ProratedPrice(h.c.Settings(ctx).DevicePrice, n, user.ExpireAt, h.now())
	if err != nil { // подписка закончилась, пока юзер выбирал
		h.devices(ctx, b, u)
		return 0, 0, false
	}
	return n, price, true
}

// devicesPick — «+N»: подтверждение цены.
func (h *handlers) devicesPick(ctx context.Context, b *bot.Bot, u *models.Update) {
	n, price, ok := h.devicePrice(ctx, b, u, devPickRoute)
	if !ok {
		return
	}
	show(ctx, b, u, keyboard.Inline(keyboard.Row(
		callbackButton(ctx, h.c, "button.yes", devYesRoute.Data(n), keyboard.StyleSuccess),
		callbackButton(ctx, h.c, "button.no", routeDevices, keyboard.StyleDanger),
	)), h.c.Text(ctx, "devices.confirm", map[string]string{"count": devicesCount(n), "price": strconv.Itoa(price)}))
}

// devicesBuy — «Да»: платёж за устройства (kind devices, срок не меняет, промокоды и аванс не действуют).
func (h *handlers) devicesBuy(ctx context.Context, b *bot.Bot, u *models.Update) {
	n, price, ok := h.devicePrice(ctx, b, u, devYesRoute)
	if !ok {
		return
	}
	userID := tgupdate.UserID(u)
	// двойное «Да»: тот же открытый платёж без чека показываем снова, второй не создаём
	if p, err := h.pay.PendingPayment(ctx, userID); err == nil && p.Kind == model.KindDevices && p.ExtraDevices == n && p.SubmittedAt == nil {
		h.showDeviceRequisites(ctx, b, u, p)
		return
	}
	p, ok := h.createPayment(ctx, b, u, model.Payment{Kind: model.KindDevices, ExtraDevices: n, Price: price})
	if !ok {
		return
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, userID), stateAwaitPhoto, nil); err != nil {
		stlog.Error(ctx, "failed to set payment state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	h.showDeviceRequisites(ctx, b, u, p)
}

func (h *handlers) showDeviceRequisites(ctx context.Context, b *bot.Bot, u *models.Update, p model.Payment) {
	set := h.c.Settings(ctx)
	show(ctx, b, u, nil, h.c.Text(ctx, "devices.requisites", map[string]string{
		"count": devicesCount(p.ExtraDevices), "price": strconv.Itoa(p.Price), "comments": commentsHTML(set.Comments),
	}))
}

func (h *handlers) devicesBusy(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, h.backKeyboard(ctx, routeMenu), h.c.Text(ctx, "devices.busy", nil))
}

// ensureUser заводит юзера в панели, если его там нет, и сразу пишет лимит устройств base+extra (новому — панельное
// умолчание не годится). Существующего не трогает: его лимит пишется только событием с устройствами.
func (h *handlers) ensureUser(ctx context.Context, id int64, username string, extra int) (*api.UserItemInfo, error) {
	user, err := h.rw.GetUser(ctx, id)
	if err == nil || !errors.Is(err, remnawave.ErrUserNotFound) {
		return user, err
	}
	user, err = h.rw.CreateUser(ctx, id, username, h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		return nil, err
	}
	if _, lerr := h.rw.SetDeviceLimit(ctx, id, h.c.Settings(ctx).DevicesBase+extra); lerr != nil {
		// юзер создан, лимит дойдёт при первом событии с устройствами; платёж из-за этого не роняем
		stlog.Warn(ctx, "failed to set device limit of a new user", "error", lerr)
	}
	return user, nil
}
