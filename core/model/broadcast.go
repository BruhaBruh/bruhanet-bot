package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/broadcast"
)

// BroadcastDraft — содержимое рассылки из админки; PhotoID — картинка в broadcast_photos, Photo — file_id Telegram у
// старых черновиков (одно из двух). Для черновика ключа нет.
type BroadcastDraft struct {
	IdempotencyKey uuid.UUID
	DraftID        uuid.UUID // отправка черновика: строка получает ключ и статус; Nil — новая
	Text, Photo    string
	PhotoID        *uuid.UUID
	Audience       string
	AudienceDays   *int
	Silent         bool
	Buttons        []broadcast.Button
	ScheduledAt    *time.Time // nil — отправить сейчас
}

// Broadcast — рассылка из админки со счётчиками доставки.
type Broadcast struct {
	ID           uuid.UUID          `json:"id"`
	Status       string             `json:"status"` // broadcast.Status*
	Text         string             `json:"text"`
	Photo        string             `json:"photo,omitempty"` // file_id старых рассылок
	PhotoID      *uuid.UUID         `json:"photoId"`
	Audience     string             `json:"audience"`
	AudienceDays *int               `json:"audienceDays"`
	Silent       bool               `json:"silent"`
	Buttons      []broadcast.Button `json:"buttons"`
	ScheduledAt  *time.Time         `json:"scheduledAt"`
	Total        int                `json:"total"`
	Sent         int                `json:"sent"`
	Failed       int                `json:"failed"`
	CreatedAt    time.Time          `json:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt"`
	FinishedAt   *time.Time         `json:"finishedAt"`
}

// BroadcastFilter — список рассылок: Statuses пуст — любые; From/To — по created_at, [From, To).
type BroadcastFilter struct {
	Statuses      []string
	From, To      time.Time
	Limit, Offset int
}

const (
	DirectPending = "pending"
	DirectSent    = "sent"
	DirectFailed  = "failed"
)

// DirectMessage — личное сообщение юзеру из админки.
type DirectMessage struct {
	ID         int64      `json:"id"`
	TelegramID int64      `json:"telegramId"`
	Text       string     `json:"text"`
	Photo      string     `json:"photo,omitempty"`
	PhotoID    *uuid.UUID `json:"photoId"`
	Status     string     `json:"status"` // pending | sent | failed
	CreatedAt  time.Time  `json:"createdAt"`
}

// AudienceCounts — получатели по аудиториям на момент запроса.
type AudienceCounts struct {
	All            int `json:"all"`
	Active         int `json:"active"`
	NoSubscription int `json:"no_subscription"`
	Expiring       int `json:"expiring"`
	Trial          int `json:"trial"`
}
