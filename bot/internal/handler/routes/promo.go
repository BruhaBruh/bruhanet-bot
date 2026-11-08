package routes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const routePromo = "promo"

var (
	havePromoRoute = callback.New[int]("have_promo")
	// buyp:<месяцы>:<id промокода>
	buyPromoRoute = callback.New2[int, int64]("buyp")
)

type promoData struct {
	Month int `json:"month"`
}

// backKeyboard — одна «Назад» под сообщением key: служебная ячейка клавиатуры «back» (подпись, стиль и место правятся
// в редакторе, куда вести — знает экран: data).
func (h *handlers) backKeyboard(ctx context.Context, key, data string) *models.InlineKeyboardMarkup {
	return h.kb(ctx, "back", key).callback("button.back", data, keyboard.StyleDefault).markup()
}

// havePromo — «Есть промокод» на экране «Верно?»: ждём код сообщением.
func (h *handlers) havePromo(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, _, ok := h.tariff(ctx, havePromoRoute, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, tgupdate.UserID(u)), stateAwaitPromo, promoData{Month: month}); err != nil {
		stlog.Error(ctx, "failed to set promo state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	show(ctx, b, u, h.backKeyboard(ctx, "promo.enter_payment", priceRoute.Data(month)), h.c.Text(ctx, "promo.enter_payment", nil))
}

// promoMenu — «Промокод» в главном меню: ждём код на дни.
func (h *handlers) promoMenu(ctx context.Context, b *bot.Bot, u *models.Update) {
	if err := fsm.Put(ctx, fsm.For(h.fsm, tgupdate.UserID(u)), stateAwaitPromoDays, nil); err != nil {
		stlog.Error(ctx, "failed to set promo state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	show(ctx, b, u, h.backKeyboard(ctx, "promo.enter_menu", routeMenu), h.c.Text(ctx, "promo.enter_menu", nil))
}

// paymentPromoText — код, присланный в await_promo: показать тариф со скидкой.
func (h *handlers) paymentPromoText(ctx context.Context, b *bot.Bot, u *models.Update) {
	if h.escapePromo(ctx, b, u) {
		return
	}
	userID := tgupdate.UserID(u)
	d, err := fsm.Data[promoData](ctx, fsm.For(h.fsm, userID))
	_, ok := h.c.Settings(ctx).Prices[d.Month]
	if err != nil || !ok { // тариф убрали из настроек, пока юзер вводил код
		h.leavePromo(ctx, userID)
		h.tariffs(ctx, b, u)
		return
	}
	p, err := h.pay.PromoByCode(ctx, u.Message.Text)
	if err == nil {
		err = h.checkPaymentPromo(ctx, p, userID)
	}
	if err != nil {
		h.rejectPromo(ctx, b, u, err, promo.PlacePayment, priceRoute.Data(d.Month))
		return
	}
	h.leavePromo(ctx, userID)
	// скидка считается от итога вместе с доп. устройствами (base — итог до скидки)
	q, err := h.quote(ctx, userID, d.Month, true)
	if err != nil {
		stlog.Error(ctx, "failed to load extra devices", "error", err)
		h.showError(ctx, b, u)
		return
	}
	base := q.Total
	vars := tariffVars(d.Month, promo.Apply(base, p))
	vars["base"] = strconv.Itoa(base)
	vars["code"] = p.Code
	send(ctx, b, tgupdate.ChatID(u), h.kb(ctx, "confirm_promo", "promo.confirm_discount").
		callback("button.yes", buyPromoRoute.Data(d.Month, p.ID), keyboard.StyleSuccess).
		callback("button.no", routePay, keyboard.StyleDanger).
		callback("button.remove_promo", priceRoute.Data(d.Month), keyboard.StyleDefault).markup(),
		h.c.Text(ctx, "promo.confirm_discount", vars))
}

// menuPromoText — код на дни из меню: продлить подписку в транзакции активации.
func (h *handlers) menuPromoText(ctx context.Context, b *bot.Bot, u *models.Update) {
	if h.escapePromo(ctx, b, u) {
		return
	}
	from := tgupdate.User(u)
	username := from.Username
	at := h.now()
	var res payflow.DaysPromoResult
	p, err := h.pay.PromoByCode(ctx, u.Message.Text)
	if err == nil {
		// ошибка панели откатывает активацию: код можно ввести снова
		res, err = payflow.ActivateDaysPromo(ctx, h.pay, h.rw, payflow.DaysPromoRequest{
			PromoID: p.ID, TelegramID: from.ID, Username: username, At: at, Squad: h.cfg.Remnawave.InternalSquadId,
			Ensure: func(ctx context.Context) (*api.UserItemInfo, error) { return h.ensureUser(ctx, from.ID, username, 0) },
		})
	}
	if errors.Is(err, payflow.ErrNotRestored) {
		stlog.Error(ctx, "days promo not restored: panel expire changed or unavailable", "error", err)
	}
	if err != nil {
		h.rejectPromo(ctx, b, u, err, promo.PlaceMenu, routeMenu)
		return
	}
	h.leavePromo(ctx, from.ID)
	send(ctx, b, tgupdate.ChatID(u), h.mainKeyboard(ctx, from.ID, "promo.days_granted"), h.c.Text(ctx, "promo.days_granted",
		map[string]string{"days": strconv.Itoa(p.Value), "expireAt": formatTime(res.NewExpire)}))
}

// buyPromo — «Да» на тарифе со скидкой. Callback подделывается: код и тариф проверяются заново.
func (h *handlers) buyPromo(ctx context.Context, b *bot.Bot, u *models.Update) {
	userID := tgupdate.UserID(u)
	f := fsm.For(h.fsm, userID)
	st, err := f.State(ctx)
	if err != nil {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	// CAS-замок от двойного «Да»: второе нажатие, пока первое в работе, ничего не делает,
	// а после него упрётся в «уже использовали» (активация пишется в ResolvePayment).
	// ponytail: убитый посреди хендлера процесс оставит promo_paying — его сбросит /start, меню или любое
	// сообщение (leavePromo); живой замок они тоже снимут, повтор 0 ₽ тогда отсечёт БД.
	if st == statePromoPaying {
		return
	}
	if err := fsm.Transition(ctx, f, st, statePromoPaying, nil); err != nil {
		if !errors.Is(err, fsm.ErrStateMismatch) {
			stlog.Error(ctx, "failed to lock promo payment", "error", err)
		}
		return
	}
	next := st
	defer func() {
		// без отмены: graceful shutdown отменяет ctx, а замок должен сняться
		ctx := context.WithoutCancel(ctx)
		if err := fsm.Transition(ctx, f, statePromoPaying, next, nil); err != nil && !errors.Is(err, fsm.ErrStateMismatch) {
			stlog.Error(ctx, "failed to unlock promo payment", "error", err)
		}
	}()

	month, promoID, err := buyPromoRoute.Parse(u.CallbackQuery.Data)
	_, ok := h.c.Settings(ctx).Prices[month]
	if err != nil || !ok {
		h.tariffs(ctx, b, u)
		return
	}
	q, err := h.quote(ctx, userID, month, true)
	if err != nil {
		stlog.Error(ctx, "failed to load extra devices", "error", err)
		h.showError(ctx, b, u)
		return
	}
	base := q.Total
	p, err := h.pay.PromoByID(ctx, promoID)
	if err == nil {
		err = h.checkPaymentPromo(ctx, p, userID)
	}
	if err != nil {
		h.rejectPromo(ctx, b, u, err, promo.PlacePayment, routePay)
		return
	}
	price := promo.Apply(base, p)
	payment, ok := h.createPayment(ctx, b, u, model.Payment{Month: month, Price: price, PromoID: &p.ID, BasePrice: &base, ExtraDevices: q.Extra})
	if !ok {
		return
	}
	if price > 0 {
		next = stateAwaitPhoto
		h.showRequisites(ctx, b, u, month, price)
		return
	}
	next = fsm.StateNone // платёж цены 0 не ждёт фото, даже если подтверждение не прошло
	h.payByPromo(ctx, b, u, payment, p)
}

// payByPromo подтверждает платёж цены 0 сразу — как админ: дата от PrevExpireAt, UpdateUser
// и уведомление юзеру через outbox в транзакции ResolvePayment.
func (h *handlers) payByPromo(ctx context.Context, b *bot.Bot, u *models.Update, pay model.Payment, p model.Promo) {
	at := h.now()
	// текст и клавиатура — до транзакции (БД текстов, HTTP в панель)
	vars := tariffVars(pay.Month, pay.Price)
	vars["expireAt"] = formatTime(payflow.ExtendedExpire(pay.PrevExpireAt, at, pay.Month))
	text := h.c.Text(ctx, "promo.paid_by_promo", vars)
	kb := h.mainKeyboard(ctx, pay.TelegramID, "promo.paid_by_promo")
	var written, old time.Time // срок, который мы пытались записать, и прежний
	squad := h.cfg.Remnawave.InternalSquadId
	_, err := h.pay.ResolvePayment(ctx, pay.ID, model.PaymentConfirmed, at, func(x model.Payment) ([]model.OutboxMessage, error) {
		expireAt := payflow.ExtendedExpire(x.PrevExpireAt, at, x.Month)
		// Лимит устройств (идемпотентный, абсолютный) — ДО срока: сбой после записи срока оставил бы продлённый срок
		// при отменённом платеже, и повтор продлил бы ещё раз.
		if err := payflow.ApplyDeviceLimit(ctx, h.rw, x, true, h.c.Settings(ctx).DevicesBase); err != nil {
			return nil, err
		}
		written, old = expireAt, x.PrevExpireAt // до вызова: ответ мог потеряться, а срок записаться
		if _, err := h.rw.UpdateUser(ctx, x.TelegramID, x.Username, expireAt, squad); err != nil {
			return nil, err
		}
		return []model.OutboxMessage{{
			EventType: payflow.EventResult,
			ChatID:    x.TelegramID,
			Text:      text,
			ParseMode: string(models.ParseModeHTML),
			Keyboard:  kb,
		}}, nil
	})
	switch {
	case errors.Is(err, store.ErrNotPending), errors.Is(err, store.ErrNotFound):
		// параллельная покупка уже отменила платёж — показываем текущее меню
		h.start(ctx, b, u)
		return
	case err != nil:
		if !written.IsZero() { // сбой после записи срока (коммит): вернуть, пока срок в панели наш
			if rerr := payflow.RestoreTerm(ctx, h.rw, pay.TelegramID, pay.Username, written, old, squad); rerr != nil {
				stlog.Error(ctx, "failed to restore panel term after promo payment failure", "error", rerr)
			}
		}
		// платёж 0 ₽ без подтверждения не нужен: фото по нему не ждём (аванса не было — панель не откатываем)
		if cerr := h.pay.CancelPayment(context.WithoutCancel(ctx), pay.ID); cerr != nil && !errors.Is(cerr, store.ErrNotPending) {
			stlog.Error(ctx, "failed to cancel promo payment", "error", cerr)
		}
		if promoErrorKey(err, promo.PlacePayment) != "" { // БД отклонила под блокировкой кода: повтор, лимит, …
			h.rejectPromo(ctx, b, u, err, promo.PlacePayment, routePay)
			return
		}
		stlog.Error(ctx, "failed to confirm promo payment", "error", err)
		h.showError(ctx, b, u)
		return
	}
	months := strconv.Itoa(pay.Month)
	observability.Payments.WithLabelValues("confirmed", months).Inc()
	send(ctx, b, h.cfg.Telegram.AdminID, nil,
		"<b>Оплата промокодом</b>",
		"",
		fmt.Sprintf(
			"Пользователь <b>%s</b> @%s (<b>%d</b>) получил подписку на <b>%d %s</b>",
			html.EscapeString(displayName(u)), html.EscapeString(pay.Username), pay.TelegramID, pay.Month, pluralMonths(pay.Month),
		),
		promoLine(p),
	)
	h.start(ctx, b, u)
}

func (h *handlers) checkPaymentPromo(ctx context.Context, p model.Promo, userID int64) error {
	// Раннее чтение, чтобы не показывать скидку, которую потом не выдадут; окончательно решает транзакция (CreatePayment).
	if blocked, err := h.pay.UserBlocked(ctx, userID); err != nil || blocked {
		if err == nil {
			err = promo.ErrBlocked
		}
		return err
	}
	used, already, err := h.pay.PromoUsage(ctx, p.ID, userID)
	if err != nil {
		return err
	}
	confirmed, err := h.pay.HasConfirmed(ctx, userID)
	if err != nil {
		return err
	}
	return promo.Check(p, used, userID, confirmed, already, promo.PlacePayment, h.now())
}

// rejectPromo показывает причину отказа (стейт ожидания кода не меняется — можно прислать другой код);
// прочие ошибки — error.generic.
func (h *handlers) rejectPromo(ctx context.Context, b *bot.Bot, u *models.Update, err error, place promo.Place, back string) {
	key := promoErrorKey(err, place)
	if key == "" {
		stlog.Error(ctx, "failed to apply promo", "error", err)
		h.showError(ctx, b, u)
		return
	}
	show(ctx, b, u, h.backKeyboard(ctx, key, back), h.c.Text(ctx, key, nil))
}

func promoErrorKey(err error, place promo.Place) string {
	switch {
	// блокировку не раскрываем: для юзера это «такого кода нет»
	case errors.Is(err, promo.ErrNotFound), errors.Is(err, store.ErrNotFound), errors.Is(err, promo.ErrBlocked):
		return "promo.not_found"
	case errors.Is(err, promo.ErrDisabled):
		return "promo.disabled"
	case errors.Is(err, promo.ErrNotStarted):
		return "promo.not_started"
	case errors.Is(err, promo.ErrExpired):
		return "promo.expired"
	case errors.Is(err, promo.ErrExhausted):
		return "promo.exhausted"
	case errors.Is(err, promo.ErrNotYours):
		return "promo.not_yours"
	case errors.Is(err, promo.ErrOnlyNew):
		return "promo.only_new"
	case errors.Is(err, promo.ErrAlreadyUsed):
		return "promo.already_used"
	case errors.Is(err, promo.ErrWrongPlace) && place == promo.PlacePayment:
		return "promo.wrong_place_payment"
	case errors.Is(err, promo.ErrWrongPlace):
		return "promo.wrong_place_menu"
	}
	return ""
}

// escapePromo: команда или не текст в ожидании кода — выход в обычный роутинг (фото — к оплате).
func (h *handlers) escapePromo(ctx context.Context, b *bot.Bot, u *models.Update) bool {
	m := u.Message
	if m.Text != "" && !strings.HasPrefix(m.Text, "/") {
		return false
	}
	if len(m.Photo) > 0 {
		h.leavePromo(ctx, tgupdate.UserID(u))
		h.photo(ctx, b, u)
		return true
	}
	h.start(ctx, b, u) // start сам выходит из ожидания кода
	return true
}

// leavePromo выходит из ожидания кода (и из promo_paying); стейт оплаты восстанавливается по pending-платежу.
func (h *handlers) leavePromo(ctx context.Context, userID int64) {
	f := fsm.For(h.fsm, userID)
	st, err := f.State(ctx)
	if err != nil {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
		return
	}
	// promo_paying — тоже: замок, оставленный упавшим процессом, иначе блокирует фото
	if !slices.Contains([]fsm.State{stateAwaitPromo, stateAwaitPromoDays, statePromoPaying, stateAwaitDevices}, st) {
		return
	}
	next := fsm.StateNone
	p, err := h.pay.PendingPayment(ctx, userID)
	switch {
	case err == nil && p.Price == 0: // платёж 0 ₽ подтверждается без фото
	case err == nil && p.SubmittedAt != nil:
		next = stateInReview
	case err == nil:
		next = stateAwaitPhoto
	case !errors.Is(err, store.ErrNotFound):
		stlog.Error(ctx, "failed to load payment", "error", err)
	}
	if err := fsm.Transition(ctx, f, st, next, nil); err != nil && !errors.Is(err, fsm.ErrStateMismatch) {
		stlog.Error(ctx, "failed to leave promo state", "error", err)
	}
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
