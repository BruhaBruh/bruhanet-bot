package routes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const trialDuration = 24 * time.Hour

func (h *handlers) photo(ctx context.Context, b *bot.Bot, u *models.Update) {
	f := fsm.From(ctx)
	st, err := f.State(ctx)
	if err != nil {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
		return
	}
	p, err := fsm.Data[model.Payment](ctx, f)
	if err != nil {
		stlog.Error(ctx, "failed to load payment", "error", err)
		return
	}
	if st == stateInReview {
		h.forwardPhoto(ctx, b, u, p.AdminMessageID)
		return
	}
	if st == stateAwaitPhoto {
		h.submitPhoto(ctx, b, u, f, p)
	}
}

func (h *handlers) submitPhoto(ctx context.Context, b *bot.Bot, u *models.Update, f *fsm.FSM, p model.Payment) {
	admin := h.cfg.Telegram.AdminID
	_, err := b.CopyMessage(ctx, &bot.CopyMessageParams{
		ChatID:     admin,
		FromChatID: u.Message.Chat.ID,
		MessageID:  u.Message.ID,
	})
	if err != nil {
		stlog.Error(ctx, "failed to copy payment photo to admin", "error", err)
		h.showError(ctx, b, u)
		return
	}
	msg := send(ctx, b, admin, nil,
		"*Покупка подписки BruhaNet*",
		"",
		fmt.Sprintf(
			"Пользователь *%s* @%s \\(*%d*\\) купил подписку на *%d %s*",
			bot.EscapeMarkdown(displayName(u)),
			bot.EscapeMarkdown(p.Username),
			p.TelegramID,
			p.Month,
			pluralMonths(p.Month),
		),
	)
	if msg == nil {
		h.showError(ctx, b, u)
		return
	}

	p.AdminMessageID = msg.ID
	if err := fsm.Transition(ctx, f, stateAwaitPhoto, stateInReview, p); err != nil {
		if errors.Is(err, fsm.ErrStateMismatch) {
			stlog.Warn(ctx, "payment photo lost the race", "error", err)
			h.closeAdminMessage(ctx, b, msg.ID, "Повторное фото этого платежа")
			return
		}
		stlog.Error(ctx, "failed to move payment to review", "error", err)
		h.closeAdminMessage(ctx, b, msg.ID, "Ошибка сохранения платежа")
		h.showError(ctx, b, u)
		return
	}
	h.grantTrial(ctx, b, u, p)
	// кнопки — только после выдачи дня: быстрый клик админа не перезапишет grantTrial
	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      admin,
		MessageID:   msg.ID,
		ReplyMarkup: h.reviewKeyboard(p.TelegramID),
	})
	if err != nil {
		stlog.Warn(ctx, "failed to attach review keyboard", "error", err)
	}
}

// grantTrial выдаёт 1 день на время проверки: от текущего срока или от now, если он в прошлом.
func (h *handlers) grantTrial(ctx context.Context, b *bot.Bot, u *models.Update, p model.Payment) {
	from := p.SubscriptionExpireAt
	if now := h.now(); now.After(from) {
		from = now
	}
	chatID := u.Message.Chat.ID
	_, err := h.rw.UpdateUser(ctx, p.TelegramID, p.Username, from.Add(trialDuration), h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		stlog.Error(ctx, "failed to grant trial day", "error", err)
		send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), "*Вы успешно отправили фото на проверку*")
		return
	}
	send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID),
		"*Вы успешно отправили фото на проверку*",
		"",
		"Вам выдан доступ на 1 день на время проверки",
	)
}

// forwardPhoto досылает админу лишнее фото платежа на проверке; юзеру не отвечаем.
func (h *handlers) forwardPhoto(ctx context.Context, b *bot.Bot, u *models.Update, adminMessageID int) {
	params := &bot.CopyMessageParams{
		ChatID:     h.cfg.Telegram.AdminID,
		FromChatID: u.Message.Chat.ID,
		MessageID:  u.Message.ID,
	}
	if adminMessageID != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: adminMessageID}
	}
	if _, err := b.CopyMessage(ctx, params); err != nil {
		stlog.Warn(ctx, "failed to forward extra payment photo", "error", err)
	}
}
