package routes

import (
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
)

// ReminderMessage — текст и клавиатура напоминания; kind "day" — за сутки, иначе — за неделю.
func ReminderMessage(kind string, expireAt time.Time, subURL string) (string, *models.InlineKeyboardMarkup) {
	text := "Ваша подписка скоро закончится"
	if kind == "day" {
		text = "Ваша подписка закончится в течение суток"
	}
	return buildMessage(
			"*Уведомление об окончании подписки*",
			"",
			text,
			"",
			"*Дата окончания подписки*",
			bot.EscapeMarkdown(formatTime(expireAt)),
		), keyboard.Inline(keyboard.Row(
			keyboard.Callback("Продлить", routePay).Style(keyboard.StyleSuccess),
			keyboard.Copy("Подписка", subURL).Style(keyboard.StylePrimary),
		))
}
