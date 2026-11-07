package routes

import (
	"context"
	"strconv"
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
	return c.Text(ctx, key, map[string]string{"expireAt": formatTime(expireAt)}), newKB(ctx, c, "reminder", key).inPanel(true).
		callback("button.renew", routePay, keyboard.StyleSuccess).
		copy("button.subscription", subURL, keyboard.StylePrimary).markup()
}

// PaymentNotice — уведомление юзеру о решении по платежу, из строки outbox: текст key с новым сроком и главная клавиатура.
// Рендерится при доставке: решение принимает и api, у которого нет ни реестра кнопок, ни клавиатур бота.
func PaymentNotice(ctx context.Context, c *content.Provider, rw Remnawave, telegramID int64, key string, expireAt time.Time, days, devices int) (string, *models.InlineKeyboardMarkup) {
	ctx = content.ForUser(ctx, telegramID)
	vars := map[string]string{"expireAt": formatTime(expireAt)}
	if days > 0 {
		vars["days"] = strconv.Itoa(days)
	}
	if devices > 0 {
		vars["devices"] = strconv.Itoa(devices)
	}
	return c.Text(ctx, key, vars), mainKeyboard(ctx, c, rw, telegramID, key, nil)
}
