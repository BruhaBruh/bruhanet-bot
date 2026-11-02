package routes

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
)

// ReminderMessage — текст (HTML) и клавиатура напоминания; kind "day" — за сутки, иначе — за неделю.
func ReminderMessage(ctx context.Context, c *content.Provider, kind string, expireAt time.Time, subURL string) (string, *models.InlineKeyboardMarkup) {
	key := "reminder.week"
	if kind == "day" {
		key = "reminder.day"
	}
	return c.Text(ctx, key, map[string]string{"expireAt": formatTime(expireAt)}), keyboard.Inline(keyboard.Row(
		keyboard.Callback(c.Button(ctx, "button.renew"), routePay).Style(keyboard.StyleSuccess),
		keyboard.Copy(c.Button(ctx, "button.subscription"), subURL).Style(keyboard.StylePrimary),
	))
}
