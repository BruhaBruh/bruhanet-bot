package routes

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func (h *handlers) confirm(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.review(ctx, b, u, confirmRoute, true)
}

func (h *handlers) decline(ctx context.Context, b *bot.Bot, u *models.Update) {
	h.review(ctx, b, u, declineRoute, false)
}

// review подтверждает или отклоняет платёж; статус решает Postgres (payflow.Resolve, та же логика у панели):
// ошибка панели откатывает транзакцию, повтор считает ту же дату.
func (h *handlers) review(ctx context.Context, b *bot.Bot, u *models.Update, route callback.Route[int64], approve bool) {
	msg := u.CallbackQuery.Message.Message
	id, err := route.Parse(u.CallbackQuery.Data)
	if err != nil || msg == nil {
		middleware.Answer(ctx, b, u, "Некорректная кнопка")
		return
	}

	p, err := payflow.Resolve(ctx, h.pay, h.rw, payflow.Decision{
		ID: id, Approve: approve, At: h.now(), Squad: h.cfg.Remnawave.InternalSquadId,
		DevicesBase: h.c.Settings(ctx).DevicesBase, ReferralDays: h.c.Settings(ctx).ReferralDays,
	})
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
	h.payReferrer(ctx, p)
	months := observability.PaymentLabel(string(p.Kind), p.Month)
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

// payReferrer — после подтверждения первой оплаты друга выдаёт дни пригласившему отдельной транзакцией. Платёж уже
// закоммичен: сбой не откатывает его, награда остаётся в referral_rewards, её дочинит сверка (internal/referral).
func (h *handlers) payReferrer(ctx context.Context, p model.Payment) {
	if !p.ReferralPending {
		return
	}
	switch _, err := payflow.PayReferrer(ctx, h.pay, h.rw, p.TelegramID, h.now(), h.cfg.Remnawave.InternalSquadId); {
	case err == nil, errors.Is(err, store.ErrAlreadyRewarded):
	case errors.Is(err, store.ErrBlocked):
		stlog.Info(ctx, "referrer bonus skipped: referrer is blocked", "reason", "blocked")
	default:
		stlog.Error(ctx, "failed to pay referrer", "error", err)
	}
}
