package model

import (
	"time"

	"github.com/go-telegram/bot/models"
)

// OutboxMessage — исходящее сообщение Telegram; payload-колонка хранит всё, кроме EventType.
type OutboxMessage struct {
	EventType string `json:"-"` // payment_result | payment_admin_closed | grant_days | reminder | broadcast | broadcast_summary | direct | text_test

	ChatID    int64                        `json:"chat_id"`
	Text      string                       `json:"text,omitempty"`
	ParseMode string                       `json:"parse_mode,omitempty"`
	Keyboard  *models.InlineKeyboardMarkup `json:"keyboard,omitempty"`
	Photo     string                       `json:"photo,omitempty"` // file_id; есть — sendPhoto с подписью Text
	// Template — Text содержит {{.переменные}}: воркер подставляет их на получателя. Нет флага — текст уходит как есть
	// (строки до этой версии, готовые тексты и обычный текст с «{{»).
	Template bool `json:"template,omitempty"`
	// Silent — без звука уведомления (disable_notification); только рассылки.
	Silent bool `json:"silent,omitempty"`

	// copy, если MessageID != 0
	FromChatID int64 `json:"from_chat_id,omitempty"`
	MessageID  int   `json:"message_id,omitempty"`

	Broadcast string `json:"broadcast,omitempty"` // uuid рассылки для счётчиков и итога
	DirectID  int64  `json:"direct_id,omitempty"` // direct_messages.id для статуса

	// TextKey — результат платежа: текст из реестра с новым сроком и главная клавиатура рендерятся при доставке
	// (строки до этой версии несут готовые Text и Keyboard).
	TextKey  string    `json:"text_key,omitempty"`
	ExpireAt time.Time `json:"expire_at,omitzero"`
	Devices  int       `json:"devices,omitempty"` // payment_result за устройства: куплено штук (переменная {{.devices}})
	Days     int       `json:"days,omitempty"`    // grant_days: сколько дней выдано (переменная {{.days}})

	// payment_admin_closed: решение из панели закрывает MessageID у админа; 0 — чека не было, закрывать нечего.
	Confirmed bool `json:"confirmed,omitempty"`
	Months    int  `json:"months,omitempty"`
	Price     int  `json:"price,omitempty"`
}

type OutboxItem struct {
	ID int64
	OutboxMessage
}
