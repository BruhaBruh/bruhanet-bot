package routes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	advanceDuration = 24 * time.Hour
	// advanceLimit — сколько неоплаченных авансов подряд допустимо; дальше аванс не выдаётся.
	advanceLimit = 3
)

func (h *handlers) photo(ctx context.Context, b *bot.Bot, u *models.Update) {
	f := fsm.From(ctx)
	st, err := f.State(ctx)
	if err != nil {
		stlog.Error(ctx, "failed to get fsm state", "error", err)
		return
	}
	p, err := h.pay.PendingPayment(ctx, f.UserID())
	if errors.Is(err, store.ErrNotFound) {
		h.start(ctx, b, u)
		return
	}
	if err != nil {
		stlog.Error(ctx, "failed to load payment", "error", err)
		h.showError(ctx, b, u)
		return
	}
	if st == stateInReview {
		h.forwardPhoto(ctx, b, u, p.AdminMessageID)
		return
	}
	if st == stateAwaitPhoto && p.SubmittedAt != nil {
		// платёж уже отправлен, но Put(in_review) не прошёл — чиним стейт, лишний раз админа не спамим
		h.forwardPhoto(ctx, b, u, p.AdminMessageID)
		if err := fsm.Put(ctx, f, stateInReview, nil); err != nil {
			stlog.Error(ctx, "failed to set review state", "error", err)
		}
		return
	}
	if st == stateAwaitPhoto {
		h.submitPhoto(ctx, b, u, f, p)
	}
}

func (h *handlers) submitPhoto(ctx context.Context, b *bot.Bot, u *models.Update, f *fsm.FSM, p model.Payment) {
	stats, err := h.pay.UserStats(ctx, p.TelegramID, p.ID)
	if err != nil {
		stlog.Error(ctx, "failed to load user stats", "error", err)
		h.showError(ctx, b, u)
		return
	}
	admin := h.cfg.Telegram.AdminID
	_, err = b.CopyMessage(ctx, &bot.CopyMessageParams{
		ChatID:     admin,
		FromChatID: u.Message.Chat.ID,
		MessageID:  u.Message.ID,
	})
	if err != nil {
		stlog.Error(ctx, "failed to copy payment photo to admin", "error", err)
		h.showError(ctx, b, u)
		return
	}
	var promoRows []string
	if p.BasePrice != nil {
		promoRows = append(promoRows, html.EscapeString(fmt.Sprintf("💳 к оплате %s ₽ (было %s ₽)",
			groupThousands(p.Price), groupThousands(*p.BasePrice))))
	}
	if p.PromoID != nil {
		pr, err := h.pay.PromoByID(ctx, *p.PromoID)
		if err != nil {
			stlog.Warn(ctx, "failed to load payment promo", "error", err)
		} else {
			promoRows = append([]string{promoLine(pr)}, promoRows...)
		}
	}
	msg := send(ctx, b, admin, nil, slices.Concat([]string{
		"<b>Покупка подписки BruhaNet</b>",
		"",
		fmt.Sprintf(
			"Пользователь <b>%s</b> @%s (<b>%d</b>) купил подписку на <b>%d %s</b>",
			html.EscapeString(displayName(u)),
			html.EscapeString(p.Username),
			p.TelegramID,
			p.Month,
			pluralMonths(p.Month),
		),
	}, promoRows, []string{""}, formatStats(stats))...)
	if msg == nil {
		h.showError(ctx, b, u)
		return
	}

	// ponytail: гонка «купить» vs «фото» может выдать день на только что отменённый платёж;
	// следующий confirm/decline вернёт prev_expire_at, MarkAdvance всё равно его посчитает.
	if err := h.pay.SubmitPayment(ctx, p.ID, msg.ID, h.now()); err != nil {
		if errors.Is(err, store.ErrNotPending) {
			stlog.Warn(ctx, "payment photo lost the race", "error", err)
			h.closeAdminMessage(ctx, b, msg.ID, "Повторное фото этого платежа")
			return
		}
		stlog.Error(ctx, "failed to submit payment", "error", err)
		h.closeAdminMessage(ctx, b, msg.ID, "Ошибка сохранения платежа")
		h.showError(ctx, b, u)
		return
	}
	observability.Payments.WithLabelValues("photo", strconv.Itoa(p.Month)).Inc()
	if stats.UnpaidAdvances < advanceLimit {
		h.grantAdvance(ctx, b, u, p)
	} else {
		send(ctx, b, u.Message.Chat.ID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent", nil))
	}
	// кнопки — только после выдачи аванса: быстрый клик админа не перезапишет grantAdvance
	_, err = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      admin,
		MessageID:   msg.ID,
		ReplyMarkup: h.reviewKeyboard(p.ID),
	})
	if err != nil {
		stlog.Warn(ctx, "failed to attach review keyboard", "error", err)
	}
	if err := fsm.Put(ctx, f, stateInReview, nil); err != nil {
		stlog.Error(ctx, "failed to set review state", "error", err)
	}
}

// grantAdvance выдаёт 1 день на время проверки: от прежнего срока или от now, если он в прошлом.
func (h *handlers) grantAdvance(ctx context.Context, b *bot.Bot, u *models.Update, p model.Payment) {
	from := p.PrevExpireAt
	if now := h.now(); now.After(from) {
		from = now
	}
	chatID := u.Message.Chat.ID
	// флаг раньше панели: если панель упадёт, перебор счётчика лучше, чем бесплатный день без учёта
	if err := h.pay.MarkAdvance(ctx, p.ID); err != nil {
		stlog.Error(ctx, "failed to mark advance", "error", err)
		send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent", nil))
		return
	}
	_, err := h.rw.UpdateUser(ctx, p.TelegramID, p.Username, from.Add(advanceDuration), h.cfg.Remnawave.InternalSquadId)
	if err != nil {
		stlog.Error(ctx, "failed to grant advance day", "error", err)
		send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent", nil))
		return
	}
	send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent_advance", nil))
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
