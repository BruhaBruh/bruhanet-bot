package model

import (
	"time"

	"github.com/google/uuid"
)

// BroadcastCopy — сообщение админа, которое `/a` копирует получателям (удаляется вместе с `/a`).
type BroadcastCopy struct {
	ChatID    int64 `json:"chatId"`
	MessageID int   `json:"messageId"`
}

// BroadcastDraft — рассылка из админки до записи в БД; Photo — file_id Telegram.
type BroadcastDraft struct {
	IdempotencyKey uuid.UUID
	Text, Photo    string
}

// Broadcast — рассылка из админки со счётчиками доставки.
type Broadcast struct {
	ID         uuid.UUID  `json:"id"`
	Text       string     `json:"text"`
	Photo      string     `json:"photo,omitempty"`
	Total      int        `json:"total"`
	Sent       int        `json:"sent"`
	Failed     int        `json:"failed"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

const (
	DirectPending = "pending"
	DirectSent    = "sent"
	DirectFailed  = "failed"
)

// DirectMessage — личное сообщение юзеру из админки.
type DirectMessage struct {
	ID         int64     `json:"id"`
	TelegramID int64     `json:"telegramId"`
	Text       string    `json:"text"`
	Photo      string    `json:"photo,omitempty"`
	Status     string    `json:"status"` // pending | sent | failed
	CreatedAt  time.Time `json:"createdAt"`
}
