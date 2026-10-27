package routes

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
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

// review подтверждает или отклоняет платёж. Порядок важен: сначала Transition (первое нажатие
// выигрывает), потом UpdateUser с абсолютной датой; при ошибке панели стейт возвращается для повтора.
func (h *handlers) review(ctx context.Context, b *bot.Bot, u *models.Update, route callback.Route[int64], approve bool) {
	msg := u.CallbackQuery.Message.Message
	telegramID, err := route.Parse(u.CallbackQuery.Data)
	if err != nil || msg == nil {
		middleware.Answer(ctx, b, u, "Некорректная кнопка")
		return
	}

	f := fsm.For(h.fsm, telegramID)
	p, err := fsm.Data[model.Payment](ctx, f)
	if err != nil && !errors.Is(err, fsm.ErrNoData) {
		stlog.Error(ctx, "failed to load payment", "error", err)
		middleware.Answer(ctx, b, u, "Ошибка, попробуйте ещё раз")
		return
	}
	if err != nil || p.AdminMessageID != msg.ID {
		middleware.Answer(ctx, b, u, "Платёж устарел")
		removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
		return
	}

	if err := fsm.Transition(ctx, f, stateInReview, fsm.StateNone, nil); err != nil {
		if errors.Is(err, fsm.ErrStateMismatch) {
			middleware.Answer(ctx, b, u, "Уже обработано")
			removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
			return
		}
		stlog.Error(ctx, "failed to close payment review", "error", err)
		middleware.Answer(ctx, b, u, "Ошибка, попробуйте ещё раз")
		return
	}

	expireAt := p.SubscriptionExpireAt
	if approve {
		if n := h.now(); n.After(expireAt) {
			expireAt = n
		}
		expireAt = expireAt.Add(time.Duration(p.Month) * monthDuration)
	}
	if _, err := h.rw.UpdateUser(ctx, p.TelegramID, p.Username, expireAt, h.cfg.Remnawave.InternalSquadId); err != nil {
		stlog.Error(ctx, "failed to update remnawave user", "error", err)
		if rerr := fsm.Transition(ctx, f, fsm.StateNone, stateInReview, p); rerr != nil {
			stlog.Error(ctx, "failed to restore payment for retry", "error", rerr)
			middleware.Answer(ctx, b, u, "Не удалось обновить пользователя, проверьте платёж вручную")
			return
		}
		middleware.Answer(ctx, b, u, "Не удалось обновить пользователя, попробуйте ещё раз")
		return
	}

	removeKeyboard(ctx, b, msg.Chat.ID, msg.ID)
	months := strconv.Itoa(p.Month)
	if approve {
		observability.Payments.WithLabelValues("confirmed", months).Inc()
		observability.PaymentsConfirmedRubles.WithLabelValues(months).Add(float64(p.Price))
		react(ctx, b, msg.Chat.ID, msg.ID, "👍")
		send(ctx, b, p.TelegramID, h.mainKeyboard(ctx, p.TelegramID),
			"*Администратор подтвердил ваш платеж*", "", "Ваша подписка была изменена")
		return
	}
	observability.Payments.WithLabelValues("declined", months).Inc()
	react(ctx, b, msg.Chat.ID, msg.ID, "👎")
	send(ctx, b, p.TelegramID, h.mainKeyboard(ctx, p.TelegramID),
		"*Администратор отклонил ваш платеж*", "", "Ваша подписка была изменена на изначальную")
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
