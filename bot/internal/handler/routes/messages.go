package routes

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

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

func send(ctx context.Context, b *bot.Bot, chatID int64, kb *models.InlineKeyboardMarkup, rows ...string) {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      buildMessage(rows...),
		ParseMode: models.ParseModeMarkdown,
	}
	if kb != nil {
		params.ReplyMarkup = kb
	}
	if _, err := b.SendMessage(ctx, params); err != nil {
		stlog.Warn(ctx, "failed to send message", "error", err)
	}
}
