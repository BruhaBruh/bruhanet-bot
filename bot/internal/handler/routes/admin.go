package routes

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// closeAdminMessage снимает кнопки с админ-сообщения и отвечает на него пометкой.
func (h *handlers) closeAdminMessage(ctx context.Context, b *bot.Bot, messageID int, note string) {
	if messageID == 0 {
		return
	}
	admin := h.cfg.Telegram.AdminID
	removeKeyboard(ctx, b, admin, messageID)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          admin,
		Text:            note,
		ReplyParameters: &models.ReplyParameters{MessageID: messageID},
	})
	if err != nil {
		stlog.Warn(ctx, "failed to reply to admin message", "error", err)
	}
}

func removeKeyboard(ctx context.Context, b *bot.Bot, chatID int64, messageID int) {
	_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
	if err != nil {
		stlog.Warn(ctx, "failed to remove keyboard", "error", err)
	}
}

func (h *handlers) reviewKeyboard(telegramID int64) *models.InlineKeyboardMarkup {
	return keyboard.Inline(keyboard.Row(
		keyboard.Callback("Подтвердить", confirmRoute.Data(telegramID)).Style(keyboard.StyleSuccess),
		keyboard.Callback("Отклонить", declineRoute.Data(telegramID)).Style(keyboard.StyleDanger),
	))
}
