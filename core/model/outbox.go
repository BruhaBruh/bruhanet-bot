package model

import "github.com/go-telegram/bot/models"

// OutboxMessage — исходящее сообщение Telegram; payload-колонка хранит всё, кроме EventType.
type OutboxMessage struct {
	EventType string `json:"-"` // payment_result | reminder | broadcast | broadcast_summary | text_test

	ChatID    int64                        `json:"chat_id"`
	Text      string                       `json:"text,omitempty"`
	ParseMode string                       `json:"parse_mode,omitempty"`
	Keyboard  *models.InlineKeyboardMarkup `json:"keyboard,omitempty"`

	// copy, если MessageID != 0
	FromChatID int64 `json:"from_chat_id,omitempty"`
	MessageID  int   `json:"message_id,omitempty"`

	Broadcast string `json:"broadcast,omitempty"` // uuid рассылки для итога
}

type OutboxItem struct {
	ID int64
	OutboxMessage
}
