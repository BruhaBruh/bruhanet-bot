package routes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
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
	show(ctx, b, u, h.tariffKeyboard(), "*Выбор тарифа*", "", "Выберите тариф")
}

func (h *handlers) tariffKeyboard() *models.InlineKeyboardMarkup {
	months := slices.Sorted(maps.Keys(h.cfg.Payment.Prices))
	buttons := make([]keyboard.Button, 0, len(months))
	for _, m := range months {
		buttons = append(buttons, keyboard.Callback(tariffTitle(m, h.cfg.Payment.Prices[m]), priceRoute.Data(m)))
	}
	rows := keyboard.Grid(2, buttons...)
	rows = append(rows, keyboard.Row(keyboard.Callback("Назад", routeMenu)))
	return keyboard.Inline(rows...)
}

// tariff разбирает месяц из callback и проверяет, что такой тариф есть: data подделывается клиентом.
func (h *handlers) tariff(route callback.Route[int], u *models.Update) (month, price int, ok bool) {
	month, err := route.Parse(u.CallbackQuery.Data)
	if err != nil {
		return 0, 0, false
	}
	price, ok = h.cfg.Payment.Prices[month]
	return month, price, ok
}

func (h *handlers) price(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(priceRoute, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	show(ctx, b, u, keyboard.Inline(keyboard.Row(
		keyboard.Callback("Да", buyRoute.Data(month)).Style(keyboard.StyleSuccess),
		keyboard.Callback("Нет", routePay).Style(keyboard.StyleDanger),
	)),
		"*Выбор тарифа*",
		"",
		fmt.Sprintf("Вы выбрали тариф на *%d %s* за *%d ₽*", month, pluralMonths(month), price),
		"",
		"Верно?",
	)
}

func (h *handlers) buy(ctx context.Context, b *bot.Bot, u *models.Update) {
	month, price, ok := h.tariff(buyRoute, u)
	if !ok {
		h.tariffs(ctx, b, u)
		return
	}
	from := tgupdate.User(u)
	username := telegramUsername(from)

	user, err := h.rw.CreateUser(ctx, from.ID, username, h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		stlog.Error(ctx, "failed to create remnawave user", "error", err)
		h.showError(ctx, b, u)
		return
	}
	f := fsm.For(h.fsm, from.ID)
	restored, err := h.cancelReview(ctx, b, f)
	if err != nil {
		stlog.Error(ctx, "failed to cancel payment in review", "error", err)
		h.showError(ctx, b, u)
		return
	}
	if restored != nil {
		user = restored
	}

	payment := model.Payment{
		TelegramID:           from.ID,
		Username:             username,
		Month:                month,
		Price:                price,
		SubscriptionExpireAt: user.ExpireAt,
	}
	if err := fsm.Put(ctx, f, stateAwaitPhoto, payment); err != nil {
		stlog.Error(ctx, "failed to save payment", "error", err)
		h.showError(ctx, b, u)
		return
	}
	observability.Payments.WithLabelValues("started", strconv.Itoa(month)).Inc()

	show(ctx, b, u, nil,
		fmt.Sprintf("*Оплата тарифа на %d %s за %d ₽*", month, pluralMonths(month), price),
		"",
		"*Для оплаты*",
		fmt.Sprintf("Переведите *%d ₽*", price),
		fmt.Sprintf("на номер `%s` в *%s*", bot.EscapeMarkdown(h.cfg.Payment.Phone), bot.EscapeMarkdown(h.cfg.Payment.Bank)),
		"с одним из комментариев:",
		"\\- `На кофе`",
		"\\- `Перекус`",
		"\\- `Одолжил`",
		"\\- `Вернул долг`",
		"и отправьте в чат фото платежа",
	)
}

// cancelReview откатывает платёж на проверке: возвращает исходный срок и закрывает админ-сообщение.
// Возвращает юзера с восстановленным сроком или nil, если откатывать нечего.
func (h *handlers) cancelReview(ctx context.Context, b *bot.Bot, f *fsm.FSM) (*api.UserItemInfo, error) {
	st, err := f.State(ctx)
	if err != nil {
		return nil, err
	}
	if st != stateInReview {
		return nil, nil
	}
	p, err := fsm.Data[model.Payment](ctx, f)
	if err != nil {
		return nil, err
	}
	if err := fsm.Transition(ctx, f, stateInReview, fsm.StateNone, nil); err != nil {
		if errors.Is(err, fsm.ErrStateMismatch) {
			return nil, nil // админ успел обработать платёж
		}
		return nil, err
	}
	user, err := h.rw.UpdateUser(ctx, p.TelegramID, p.Username, p.SubscriptionExpireAt, h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		if rerr := fsm.Transition(ctx, f, fsm.StateNone, stateInReview, p); rerr != nil {
			stlog.Error(ctx, "failed to restore payment after cancel failure", "error", rerr)
		}
		return nil, err
	}
	h.closeAdminMessage(ctx, b, p.AdminMessageID, "Отменён пользователем")
	return user, nil
}
