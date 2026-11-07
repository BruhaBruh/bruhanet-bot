package routes

import (
	"context"
	"errors"
	"html"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
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

// tariffsKey / confirmKey — сообщение, под которым стоит клавиатура экрана.
var (
	tariffsKey = map[string]string{"tariffs": "payment.tariffs", "gift_tariffs": "gift.tariffs"}
	confirmKey = map[string]string{"confirm": "payment.confirm", "confirm_devices": "payment.confirm_devices"}
)

func (h *handlers) tariffs(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, h.tariffKeyboard(ctx), h.c.Text(ctx, "payment.tariffs", nil))
}

func (h *handlers) tariffKeyboard(ctx context.Context) *models.InlineKeyboardMarkup {
	return h.tariffKeyboardFor(ctx, "tariffs", priceRoute)
}

// tariffKeyboardFor — клавиатура kb с кнопками тарифов (блок @tariffs), ведущими на route (покупка тарифа или подарок).
func (h *handlers) tariffKeyboardFor(ctx context.Context, kb string, route callback.Route[int]) *models.InlineKeyboardMarkup {
	prices := h.c.Settings(ctx).Prices
	months := slices.Sorted(maps.Keys(prices))
	buttons := make([]keyboard.Button, 0, len(months))
	for _, m := range months {
		buttons = append(buttons, keyboard.Callback(h.c.Text(ctx, "button.tariff", tariffVars(m, prices[m])), route.Data(m)).
			Style(h.c.Style(ctx, "button.tariff", keyboard.StyleDefault)))
	}
	return h.kb(ctx, kb, tariffsKey[kb]).block("@tariffs", keyboard.Grid(2, buttons...)).
		callback("button.back", routeMenu, keyboard.StyleDefault).markup()
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
	userID := tgupdate.UserID(u)
	h.leavePromo(ctx, userID) // «Назад»/«Убрать промокод» из ожидания кода
	q, err := h.quote(ctx, userID, month, true)
	if err != nil {
		stlog.Error(ctx, "failed to load extra devices", "error", err)
		h.showError(ctx, b, u)
		return
	}
	confirm := func(id string) *kbBuilder {
		return h.kb(ctx, id, confirmKey[id]).
			callback("button.yes", buyRoute.Data(month), keyboard.StyleSuccess).
			callback("button.no", routePay, keyboard.StyleDanger).
			callback("button.have_promo", havePromoRoute.Data(month), keyboard.StyleDefault)
	}
	if q.hasPaidExtra() {
		show(ctx, b, u, confirm("confirm_devices").
			callback("button.drop_devices", buyNoDevRoute.Data(month), keyboard.StyleDefault).markup(),
			h.c.Text(ctx, "payment.confirm_devices", q.vars(h.c.Settings(ctx).DevicePrice)))
		return
	}
	show(ctx, b, u, confirm("confirm").markup(), h.c.Text(ctx, "payment.confirm", tariffVars(month, price)))
}

func (h *handlers) buy(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.buyTariff(ctx, b, u, buyRoute, true)
}

// buyNoDevices — продление без доп. устройств: цена только тарифа, подтверждение сбросит устройства до базы.
func (h *handlers) buyNoDevices(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.buyTariff(ctx, b, u, buyNoDevRoute, false)
}

// buyTariff создаёт платёж подписки. Цена и число доп. устройств фиксируются сейчас и сохраняются в платеже:
// подтверждение применяет их, а не пересчитывает.
func (h *handlers) buyTariff(ctx context.Context, b *bot.Bot, u *models.Update, route callback.Route[int], keepExtra bool) {
	month, _, ok := h.tariff(ctx, route, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	q, err := h.quote(ctx, tgupdate.UserID(u), month, keepExtra)
	if err != nil {
		stlog.Error(ctx, "failed to load extra devices", "error", err)
		h.showError(ctx, b, u)
		return
	}
	if _, ok := h.createPayment(ctx, b, u, model.Payment{Month: month, Price: q.Total, ExtraDevices: q.Extra}); !ok {
		return
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, tgupdate.UserID(u)), stateAwaitPhoto, nil); err != nil {
		stlog.Error(ctx, "failed to set payment state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	h.showRequisites(ctx, b, u, month, q.Total)
}

// createPayment заводит юзера в панели и pending-платёж np (тариф и цена — от вызывающего);
// false — ошибка уже показана юзеру.
func (h *handlers) createPayment(ctx context.Context, b *bot.Bot, u *models.Update, np model.Payment) (model.Payment, bool) {
	from := tgupdate.User(u)
	username := from.Username

	np.TelegramID, np.Username = from.ID, username
	// Подарок срок покупателя не читает и не трогает: юзера в панели не заводим.
	var readPrev func(ctx context.Context) (time.Time, error)
	if np.Kind != model.KindGift {
		readPrev = func(ctx context.Context) (time.Time, error) {
			// Срок читается под замком юзера в хранилище (после параллельной выдачи дней); CreateUser заводит юзера, если его нет.
			user, err := h.ensureUser(ctx, from.ID, username, np.ExtraDevices)
			if err != nil {
				stlog.Error(ctx, "failed to create remnawave user", "error", err)
				return time.Time{}, err
			}
			return user.ExpireAt, nil
		}
	}
	// Старый pending отменяется в той же транзакции; выданный по нему аванс откатываем в панели.
	p, cancelled, err := h.pay.CreatePayment(ctx, np, func(old model.Payment) error {
		if !old.AdvanceGranted {
			return nil
		}
		_, err := h.rw.UpdateUser(ctx, old.TelegramID, old.Username, old.PrevExpireAt, h.cfg.Remnawave.InternalSquadId)
		return err
	}, readPrev)
	if errors.Is(err, promo.ErrBlocked) { // скидка заблокированному не выдаётся; для юзера — «кода нет»
		h.rejectPromo(ctx, b, u, err, promo.PlacePayment, routePay)
		return model.Payment{}, false
	}
	if errors.Is(err, store.ErrPendingExists) { // докупка устройств или подарок при открытой оплате
		busy := np.Kind
		if busy == "" || busy == model.KindTariff { // тариф упёрся в оплаченную докупку/подарок с отправленным чеком
			if old, perr := h.pay.PendingPayment(ctx, np.TelegramID); perr == nil {
				busy = old.Kind
			}
		}
		if busy == model.KindGift {
			h.giftBusy(ctx, b, u)
		} else {
			h.devicesBusy(ctx, b, u)
		}
		return model.Payment{}, false
	}
	if err != nil {
		stlog.Error(ctx, "failed to create payment", "error", err)
		h.showError(ctx, b, u)
		return model.Payment{}, false
	}
	if cancelled != nil {
		h.closeAdminMessage(ctx, b, cancelled.AdminMessageID, "Отменён пользователем")
	}
	observability.Payments.WithLabelValues("started", observability.PaymentLabel(string(np.Kind), np.Month)).Inc()
	return p, true
}

func (h *handlers) showRequisites(ctx context.Context, b *bot.Bot, u *models.Update, month, price int) {
	set := h.c.Settings(ctx)
	vars := tariffVars(month, price)
	vars["comments"] = commentsHTML(set.Comments)
	show(ctx, b, u, h.kb(ctx, "", "payment.requisites").markup(), h.c.Text(ctx, "payment.requisites", vars))
}

// commentsHTML — Raw-переменная comments: строка «- <code>…</code>» на комментарий.
func commentsHTML(comments []string) string {
	rows := make([]string, len(comments))
	for i, c := range comments {
		rows[i] = "- <code>" + html.EscapeString(c) + "</code>"
	}
	return strings.Join(rows, "\n")
}
