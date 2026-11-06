package routes

import (
	"context"
	"html"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

var (
	priceRoute = callback.New[int]("price")
	buyRoute   = callback.New[int]("buy")
)

func (h *handlers) tariffs(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, h.tariffKeyboard(ctx), h.c.Text(ctx, "payment.tariffs", nil))
}

func (h *handlers) tariffKeyboard(ctx context.Context) *models.InlineKeyboardMarkup {
	prices := h.c.Settings(ctx).Prices
	months := slices.Sorted(maps.Keys(prices))
	buttons := make([]keyboard.Button, 0, len(months))
	for _, m := range months {
		buttons = append(buttons, keyboard.Callback(h.c.Text(ctx, "button.tariff", tariffVars(m, prices[m])), priceRoute.Data(m)).
			Style(h.c.Style(ctx, "button.tariff", keyboard.StyleDefault)))
	}
	rows := keyboard.Grid(2, buttons...)
	rows = append(rows, keyboard.Row(callbackButton(ctx, h.c, "button.back", routeMenu, keyboard.StyleDefault)))
	return keyboard.Inline(rows...)
}

// tariff разбирает месяц из callback и проверяет, что такой тариф есть в текущих настройках: data подделывается клиентом.
func (h *handlers) tariff(ctx context.Context, route callback.Route[int], u *models.Update) (month, price int, ok bool) {
	month, err := route.Parse(u.CallbackQuery.Data)
	if err != nil {
		return 0, 0, false
	}
	price, ok = h.c.Settings(ctx).Prices[month]
	return month, price, ok
}

func (h *handlers) price(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(ctx, priceRoute, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	h.leavePromo(ctx, tgupdate.UserID(u)) // «Назад»/«Убрать промокод» из ожидания кода
	show(ctx, b, u, keyboard.Inline(
		keyboard.Row(
			callbackButton(ctx, h.c, "button.yes", buyRoute.Data(month), keyboard.StyleSuccess),
			callbackButton(ctx, h.c, "button.no", routePay, keyboard.StyleDanger),
		),
		keyboard.Row(callbackButton(ctx, h.c, "button.have_promo", havePromoRoute.Data(month), keyboard.StyleDefault)),
	), h.c.Text(ctx, "payment.confirm", tariffVars(month, price)))
}

func (h *handlers) buy(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(ctx, buyRoute, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	if _, ok := h.createPayment(ctx, b, u, model.Payment{Month: month, Price: price}); !ok {
		return
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, tgupdate.UserID(u)), stateAwaitPhoto, nil); err != nil {
		stlog.Error(ctx, "failed to set payment state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	h.showRequisites(ctx, b, u, month, price)
}

// createPayment заводит юзера в панели и pending-платёж np (тариф и цена — от вызывающего);
// false — ошибка уже показана юзеру.
func (h *handlers) createPayment(ctx context.Context, b *bot.Bot, u *models.Update, np model.Payment) (model.Payment, bool) {
	from := tgupdate.User(u)
	username := telegramUsername(from)

	user, err := h.rw.CreateUser(ctx, from.ID, username, h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		stlog.Error(ctx, "failed to create remnawave user", "error", err)
		h.showError(ctx, b, u)
		return model.Payment{}, false
	}
	np.TelegramID, np.Username, np.PrevExpireAt = from.ID, username, user.ExpireAt
	// Старый pending отменяется в той же транзакции; выданный по нему аванс откатываем в панели.
	p, cancelled, err := h.pay.CreatePayment(ctx, np, func(old model.Payment) error {
		if !old.AdvanceGranted {
			return nil
		}
		_, err := h.rw.UpdateUser(ctx, old.TelegramID, old.Username, old.PrevExpireAt, h.cfg.Remnawave.InternalSquadId)
		return err
	})
	if err != nil {
		stlog.Error(ctx, "failed to create payment", "error", err)
		h.showError(ctx, b, u)
		return model.Payment{}, false
	}
	if cancelled != nil {
		h.closeAdminMessage(ctx, b, cancelled.AdminMessageID, "Отменён пользователем")
	}
	observability.Payments.WithLabelValues("started", strconv.Itoa(np.Month)).Inc()
	return p, true
}

func (h *handlers) showRequisites(ctx context.Context, b *bot.Bot, u *models.Update, month, price int) {
	set := h.c.Settings(ctx)
	vars := tariffVars(month, price)
	vars["comments"] = commentsHTML(set.Comments)
	show(ctx, b, u, nil, h.c.Text(ctx, "payment.requisites", vars))
}

// commentsHTML — Raw-переменная comments: строка «- <code>…</code>» на комментарий.
func commentsHTML(comments []string) string {
	rows := make([]string, len(comments))
	for i, c := range comments {
		rows[i] = "- <code>" + html.EscapeString(c) + "</code>"
	}
	return strings.Join(rows, "\n")
}
