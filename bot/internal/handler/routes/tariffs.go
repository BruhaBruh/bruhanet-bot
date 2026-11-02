package routes

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"

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
	// Старый pending отменяется в той же транзакции; выданный по нему аванс откатываем в панели.
	_, cancelled, err := h.pay.CreatePayment(ctx, model.Payment{
		TelegramID:   from.ID,
		Username:     username,
		Month:        month,
		Price:        price,
		PrevExpireAt: user.ExpireAt,
	}, func(old model.Payment) error {
		if !old.AdvanceGranted {
			return nil
		}
		_, err := h.rw.UpdateUser(ctx, old.TelegramID, old.Username, old.PrevExpireAt, h.cfg.Remnawave.InternalSquadId)
		return err
	})
	if err != nil {
		stlog.Error(ctx, "failed to create payment", "error", err)
		h.showError(ctx, b, u)
		return
	}
	if cancelled != nil {
		h.closeAdminMessage(ctx, b, cancelled.AdminMessageID, "Отменён пользователем")
	}
	if err := fsm.Put(ctx, fsm.For(h.fsm, from.ID), stateAwaitPhoto, nil); err != nil {
		stlog.Error(ctx, "failed to set payment state", "error", err)
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
