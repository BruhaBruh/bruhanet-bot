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

const routePromo = "promo"

var (
	havePromoRoute = callback.New[int]("have_promo")
	// buyp:<месяцы>:<id промокода>
	buyPromoRoute = callback.New2[int, int64]("buyp")
)

type promoData struct {
	Month int `json:"month"`
}

func (h *handlers) backKeyboard(ctx context.Context, data string) *models.InlineKeyboardMarkup {
	return keyboard.Inline(keyboard.Row(callbackButton(ctx, h.c, "button.back", data, keyboard.StyleDefault)))
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
	show(ctx, b, u, h.backKeyboard(ctx, priceRoute.Data(month)), h.c.Text(ctx, "promo.enter_payment", nil))
}

// promoMenu — «Промокод» в главном меню: ждём код на дни.
func (h *handlers) promoMenu(ctx context.Context, b *bot.Bot, u *models.Update) {
	if err := fsm.Put(ctx, fsm.For(h.fsm, tgupdate.UserID(u)), stateAwaitPromoDays, nil); err != nil {
		stlog.Error(ctx, "failed to set promo state", "error", err)
		h.showError(ctx, b, u)
		return
	}
	show(ctx, b, u, h.backKeyboard(ctx, routeMenu), h.c.Text(ctx, "promo.enter_menu", nil))
}

// paymentPromoText — код, присланный в await_promo: показать тариф со скидкой.
func (h *handlers) paymentPromoText(ctx context.Context, b *bot.Bot, u *models.Update) {
	if h.escapePromo(ctx, b, u) {
		return
	}
	userID := tgupdate.UserID(u)
	d, err := fsm.Data[promoData](ctx, fsm.For(h.fsm, userID))
	base, ok := h.c.Settings(ctx).Prices[d.Month]
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
	vars := tariffVars(d.Month, promo.Apply(base, p))
	vars["base"] = strconv.Itoa(base)
	vars["code"] = p.Code
	send(ctx, b, tgupdate.ChatID(u), keyboard.Inline(
		keyboard.Row(
			callbackButton(ctx, h.c, "button.yes", buyPromoRoute.Data(d.Month, p.ID), keyboard.StyleSuccess),
			callbackButton(ctx, h.c, "button.no", routePay, keyboard.StyleDanger),
		),
		keyboard.Row(callbackButton(ctx, h.c, "button.remove_promo", priceRoute.Data(d.Month), keyboard.StyleDefault)),
	), h.c.Text(ctx, "promo.confirm_discount", vars))
}

// menuPromoText — код на дни из меню: продлить подписку в транзакции активации.
func (h *handlers) menuPromoText(ctx context.Context, b *bot.Bot, u *models.Update) {
	if h.escapePromo(ctx, b, u) {
		return
	}
	from := tgupdate.User(u)
	username := telegramUsername(from)
	at := h.now()
	var expireAt, oldExpireAt time.Time
	p, err := h.pay.PromoByCode(ctx, u.Message.Text)
	if err == nil {
		// ошибка панели откатывает активацию: код можно ввести снова
		err = h.pay.ActivateDaysPromo(ctx, p.ID, from.ID, at, func(p model.Promo) error {
			user, err := h.rw.CreateUser(ctx, from.ID, username, h.cfg.Remnawave.InternalSquadId)
			if err != nil {
				return err
			}
			newExpireAt := later(user.ExpireAt, at).Add(time.Duration(p.Value) * 24 * time.Hour)
			if _, err = h.rw.UpdateUser(ctx, from.ID, username, newExpireAt, h.cfg.Remnawave.InternalSquadId); err != nil {
				return err
			}
			oldExpireAt, expireAt = user.ExpireAt, newExpireAt
			return nil
		})
	}
	if err != nil && !expireAt.IsZero() {
		// панель продлена, а коммит не прошёл — возвращаем прежний срок,
		// только если срок в панели всё ещё наш (иначе его успел поменять админ — не затираем).
		// ponytail: GetUser→UpdateUser не атомарен; окно — миллисекунды после редкого сбоя коммита.
		cctx := context.WithoutCancel(ctx)
		if cur, gerr := h.rw.GetUser(cctx, from.ID); gerr != nil || !cur.ExpireAt.Equal(expireAt) {
			stlog.Error(ctx, "days promo not restored: panel expire changed or unavailable",
				"error", gerr, "expire_at", oldExpireAt)
		} else if _, cerr := h.rw.UpdateUser(cctx, from.ID, username, oldExpireAt,
			h.cfg.Remnawave.InternalSquadId); cerr != nil {
			stlog.Error(ctx, "failed to restore expire after days promo", "error", cerr, "expire_at", oldExpireAt)
		}
	}
	if err != nil {
		h.rejectPromo(ctx, b, u, err, promo.PlaceMenu, routeMenu)
		return
	}
	h.leavePromo(ctx, from.ID)
	send(ctx, b, tgupdate.ChatID(u), h.mainKeyboard(ctx, from.ID), h.c.Text(ctx, "promo.days_granted",
		map[string]string{"days": strconv.Itoa(p.Value), "expireAt": formatTime(expireAt)}))
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
	base, ok := h.c.Settings(ctx).Prices[month]
	if err != nil || !ok {
		h.tariffs(ctx, b, u)
		return
	}
	p, err := h.pay.PromoByID(ctx, promoID)
	if err == nil {
		err = h.checkPaymentPromo(ctx, p, userID)
	}
	if err != nil {
		h.rejectPromo(ctx, b, u, err, promo.PlacePayment, routePay)
		return
	}
	price := promo.Apply(base, p)
	payment, ok := h.createPayment(ctx, b, u, model.Payment{Month: month, Price: price, PromoID: &p.ID, BasePrice: &base})
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
	vars["expireAt"] = formatTime(extendedExpire(pay.PrevExpireAt, at, pay.Month))
	text := h.c.Text(ctx, "promo.paid_by_promo", vars)
	kb := h.mainKeyboard(ctx, pay.TelegramID)
	_, err := h.pay.ResolvePayment(ctx, pay.ID, model.PaymentConfirmed, at, func(x model.Payment) (model.OutboxMessage, error) {
		expireAt := extendedExpire(x.PrevExpireAt, at, x.Month)
		if _, err := h.rw.UpdateUser(ctx, x.TelegramID, x.Username, expireAt, h.cfg.Remnawave.InternalSquadId); err != nil {
			return model.OutboxMessage{}, err
		}
		return model.OutboxMessage{
			EventType: "payment_result",
			ChatID:    x.TelegramID,
			Text:      text,
			ParseMode: string(models.ParseModeHTML),
			Keyboard:  kb,
		}, nil
	})
	switch {
	case errors.Is(err, store.ErrNotPending), errors.Is(err, store.ErrNotFound):
		// параллельная покупка уже отменила платёж — показываем текущее меню
		h.start(ctx, b, u)
		return
	case err != nil:
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
	show(ctx, b, u, h.backKeyboard(ctx, back), h.c.Text(ctx, key, nil))
}

func promoErrorKey(err error, place promo.Place) string {
	switch {
	case errors.Is(err, promo.ErrNotFound), errors.Is(err, store.ErrNotFound):
		return "promo.not_found"
	case errors.Is(err, promo.ErrDisabled):
		return "promo.disabled"
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
	if !slices.Contains([]fsm.State{stateAwaitPromo, stateAwaitPromoDays, statePromoPaying}, st) {
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

// extendedExpire — срок после подтверждения оплаты: от прежнего срока или от at, если он в прошлом.
func extendedExpire(prev, at time.Time, months int) time.Time {
	return later(prev, at).Add(time.Duration(months) * monthDuration)
}
