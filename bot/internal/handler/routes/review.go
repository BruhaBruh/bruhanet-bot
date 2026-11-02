package routes

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const monthDuration = 30 * 24 * time.Hour

func (h *handlers) confirm(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.review(ctx, b, u, confirmRoute, true)
}

func (h *handlers) decline(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.review(ctx, b, u, declineRoute, false)
}

// review подтверждает или отклоняет платёж. Статус решает Postgres: ResolvePayment под блокировкой
// строки вызывает UpdateUser с абсолютной датой от PrevExpireAt и кладёт уведомление юзеру в outbox;
// ошибка панели откатывает транзакцию, повтор считает ту же дату.
func (h *handlers) review(ctx context.Context, b *bot.Bot, u *models.Update, route callback.Route[int64], approve bool) {
	msg := u.CallbackQuery.Message.Message
	id, err := route.Parse(u.CallbackQuery.Data)
	if err != nil || msg == nil {
		middleware.Answer(ctx, b, u, "Некорректная кнопка")
		return
	}

	status := model.PaymentDeclined
	if approve {
		status = model.PaymentConfirmed
	}
	at := h.now()
	// клавиатура (HTTP в панель) — вне транзакции ResolvePayment; юзер в панели к review уже есть
	var kb *models.InlineKeyboardMarkup
	cur, err := h.pay.PaymentByID(ctx, id)
	if err == nil {
		kb = h.mainKeyboard(ctx, cur.TelegramID)
	}
	var p model.Payment
	if err == nil {
		p, err = h.pay.ResolvePayment(ctx, id, status, at, func(p model.Payment) (model.OutboxMessage, error) {
			expireAt := p.PrevExpireAt
			rows := []string{"*Администратор отклонил ваш платеж*", "", "Ваша подписка была изменена на изначальную"}
			if approve {
				if at.After(expireAt) {
					expireAt = at
				}
				expireAt = expireAt.Add(time.Duration(p.Month) * monthDuration)
				rows = []string{"*Администратор подтвердил ваш платеж*", "", "Ваша подписка была изменена"}
			}
			if _, err := h.rw.UpdateUser(ctx, p.TelegramID, p.Username, expireAt, h.cfg.Remnawave.InternalSquadId); err != nil {
				return model.OutboxMessage{}, err
			}
			return model.OutboxMessage{
				EventType: "payment_result",
				ChatID:    p.TelegramID,
				Text:      buildMessage(rows...),
				ParseMode: string(models.ParseModeMarkdown),
				Keyboard:  kb,
			}, nil
		})
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		middleware.Answer(ctx, b, u, "Платёж устарел")
		removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
		return
	case errors.Is(err, store.ErrNotPending):
		middleware.Answer(ctx, b, u, "Уже обработано")
		removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
		return
	case err != nil:
		stlog.Error(ctx, "failed to resolve payment", "error", err)
		middleware.Answer(ctx, b, u, "Не удалось обновить пользователя, попробуйте ещё раз")
		return
	}

	removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
	months := strconv.Itoa(p.Month)
	if approve {
		observability.Payments.WithLabelValues("confirmed", months).Inc()
		observability.PaymentsConfirmedRubles.WithLabelValues(months).Add(float64(p.Price))
		react(ctx, b, msg.Chat.ID, msg.ID, "👍")
	} else {
		observability.Payments.WithLabelValues("declined", months).Inc()
		react(ctx, b, msg.Chat.ID, msg.ID, "👎")
	}
	// стейт — лишь подсказка роутинга; если юзер уже ушёл из in_review, не трогаем
	err = fsm.Transition(ctx, fsm.For(h.fsm, p.TelegramID), stateInReview, fsm.StateNone, nil)
	if err != nil && !errors.Is(err, fsm.ErrStateMismatch) {
		stlog.Error(ctx, "failed to reset review state", "error", err)
	}
}

func react(ctx context.Context, b *bot.Bot, chatID int64, messageID int, emoji string) {
	_, err := b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    chatID,
		MessageID: messageID,
		Reaction: []models.ReactionType{{
			Type:              models.ReactionTypeTypeEmoji,
			ReactionTypeEmoji: &models.ReactionTypeEmoji{Type: models.ReactionTypeTypeEmoji, Emoji: emoji},
		}},
		IsBig: bot.True(),
	})
	if err != nil {
		stlog.Warn(ctx, "failed to set message reaction", "error", err)
	}
}
