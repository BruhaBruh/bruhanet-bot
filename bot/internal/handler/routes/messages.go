package routes

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const errorText = "К сожалению, произошла ошибка, обратитесь в поддержку"

func buildMessage(rows ...string) string {
	return strings.Join(rows, "\n")
}

// displayName никогда не возвращает пустую строку: пустой *bold* ломает MarkdownV2.
func displayName(u *models.Update) string {
	user := tgupdate.User(u)
	switch {
	case user == nil:
		return "друг"
	case user.FirstName != "":
		return user.FirstName
	case user.Username != "":
		return user.Username
	}
	return fmt.Sprintf("#%d", user.ID)
}

// telegramUsername — имя для панели и админа: username, без него — ID.
func telegramUsername(user *models.User) string {
	if user.Username != "" {
		return user.Username
	}
	return fmt.Sprintf("%d", user.ID)
}

func send(ctx context.Context, b *bot.Bot, chatID int64, kb *models.InlineKeyboardMarkup, rows ...string) *models.Message {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      buildMessage(rows...),
		ParseMode: models.ParseModeMarkdown,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	msg, err := b.SendMessage(ctx, params)
	if err != nil {
		stlog.Warn(ctx, "failed to send message", "error", err)
		return nil
	}
	return msg
}

// show редактирует сообщение колбэка на месте, иначе отправляет новое.
func show(ctx context.Context, b *bot.Bot, u *models.Update, kb *models.InlineKeyboardMarkup, rows ...string) {
	if u.CallbackQuery != nil && u.CallbackQuery.Message.Message != nil {
		m := u.CallbackQuery.Message.Message
		params := &bot.EditMessageTextParams{
			ChatID:    m.Chat.ID,
			MessageID: m.ID,
			Text:      buildMessage(rows...),
			ParseMode: models.ParseModeMarkdown,
		}
		if kb != nil {
			params.ReplyMarkup = kb
		}
		_, err := b.EditMessageText(ctx, params)
		if err == nil || strings.Contains(err.Error(), "message is not modified") {
			return
		}
		stlog.Warn(ctx, "failed to edit message, sending new one", "error", err)
	}
	send(ctx, b, tgupdate.ChatID(u), kb, rows...)
}

func (h *handlers) showError(ctx context.Context, b *bot.Bot, u *models.Update) {
	show(ctx, b, u, keyboard.Inline(
		keyboard.Row(keyboard.Callback("Назад", routeMenu)),
		keyboard.Row(keyboard.URL("Поддержка", h.cfg.Support.URL)),
	), errorText)
}
