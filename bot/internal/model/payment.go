package model

import "time"

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentConfirmed PaymentStatus = "confirmed"
	PaymentDeclined  PaymentStatus = "declined"
	PaymentCancelled PaymentStatus = "cancelled"
)

type Payment struct {
	ID                   int64     `json:"-"`
	TelegramID           int64     `json:"telegramId"`
	Username             string    `json:"username"`
	Month                int       `json:"month"`
	Price                int       `json:"price"`
	SubscriptionExpireAt time.Time `json:"subscriptionExpireAt"`
	AdminMessageID       int       `json:"adminMessageId,omitempty"`

	Status         PaymentStatus `json:"-"`
	PrevExpireAt   time.Time     `json:"-"`
	AdvanceGranted bool          `json:"-"`
	CreatedAt      time.Time     `json:"-"`
	SubmittedAt    *time.Time    `json:"-"`
	ResolvedAt     *time.Time    `json:"-"`
}

type UserStats struct {
	Confirmed, ConfirmedSum, UnpaidAdvances int
	LastConfirmedAt, FirstConfirmedAt       *time.Time
	// HasHistory — есть платежи кроме excludeID.
	HasHistory bool
}
