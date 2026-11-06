package routes

import (
	"cmp"
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
	handle := ""
	if p.Username != "" {
		handle = " @" + html.EscapeString(p.Username)
	}
	limit := h.c.Settings(ctx).AdvanceLimit
	head := []string{
		"<b>Покупка подписки BruhaNet</b>",
		"",
		fmt.Sprintf(
			"Пользователь <b>%s</b>%s (<b>%d</b>) купил подписку на <b>%d %s</b>",
			html.EscapeString(displayName(u)),
			handle,
			p.TelegramID,
			p.Month,
			pluralMonths(p.Month),
		),
	}
	if p.Kind == model.KindDevices {
		head = []string{
			"<b>Покупка устройств BruhaNet</b>",
			"",
			fmt.Sprintf(
				"Пользователь <b>%s</b>%s (<b>%d</b>) докупает <b>%s</b> за <b>%s ₽</b>. Срок подписки не меняется",
				html.EscapeString(displayName(u)), handle, p.TelegramID, devicesCount(p.ExtraDevices), groupThousands(p.Price),
			),
		}
	} else if p.Kind == model.KindGift {
		head = []string{
			"<b>Покупка подарка BruhaNet</b>",
			"",
			fmt.Sprintf(
				"Пользователь <b>%s</b>%s (<b>%d</b>) покупает подарочную подписку на <b>%d %s</b> за <b>%s ₽</b>. Срок покупателя не меняется",
				html.EscapeString(displayName(u)), handle, p.TelegramID, p.Month, pluralMonths(p.Month), groupThousands(p.Price),
			),
		}
	} else if p.ExtraDevices > 0 {
		head = append(head, fmt.Sprintf("🖥 доп. устройств в тарифе: %d", p.ExtraDevices))
	}
	msg := send(ctx, b, admin, nil, slices.Concat(head, promoRows, []string{""}, formatStats(stats, limit))...)
	if msg == nil {
		h.showError(ctx, b, u)
		return
	}

	if err := h.pay.SubmitPayment(ctx, p.ID, msg.ID, largestPhoto(u.Message.Photo), h.now()); err != nil {
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
	// Аванс — день подписки на время проверки; за докупку устройств и подарок срока нет, аванса тоже.
	if stats.UnpaidAdvances < limit && p.Kind == model.KindTariff {
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

// largestPhoto — file_id самого большого размера; роут фото гарантирует непустой список.
func largestPhoto(sizes []models.PhotoSize) string {
	return slices.MaxFunc(sizes, func(a, b models.PhotoSize) int {
		return cmp.Compare(a.Width*a.Height, b.Width*b.Height)
	}).FileID
}

// grantAdvance выдаёт аванс на время проверки: от прежнего срока или от now, если он в прошлом. Запись идёт под
// блокировкой платежа: решение из панели (кнопок оно не ждёт) или отмена новой покупкой не будут затёрты авансом.
func (h *handlers) grantAdvance(ctx context.Context, b *bot.Bot, u *models.Update, p model.Payment) {
	chatID := u.Message.Chat.ID
	advance := time.Duration(h.c.Settings(ctx).AdvanceHours) * time.Hour
	var panelErr error
	err := h.pay.GrantAdvance(ctx, p.ID, func(locked model.Payment) {
		_, panelErr = h.rw.UpdateUser(ctx, locked.TelegramID, locked.Username, later(locked.PrevExpireAt, h.now()).Add(advance), h.cfg.Remnawave.InternalSquadId)
	})
	switch {
	case errors.Is(err, store.ErrNotPending):
		stlog.Warn(ctx, "payment resolved before advance", "error", err)
	case errors.Is(err, store.ErrBlocked):
		// заблокирован админом: аванса нет, юзеру обычное «фото на проверке» (о блокировке не сообщаем)
	case err != nil:
		stlog.Error(ctx, "failed to mark advance", "error", err)
	case panelErr != nil:
		stlog.Error(ctx, "failed to grant advance day", "error", panelErr)
	default:
		send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent_advance", map[string]string{"advance": advanceText(h.c.Settings(ctx).AdvanceHours)}))
		return
	}
	send(ctx, b, chatID, h.mainKeyboard(ctx, p.TelegramID), h.c.Text(ctx, "payment.photo_sent", nil))
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
