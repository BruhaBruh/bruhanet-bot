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
	ID             int64
	TelegramID     int64
	Username       string
	Month          int
	Price          int
	Status         PaymentStatus
	PrevExpireAt   time.Time
	AdvanceGranted bool
	AdminMessageID int
	CreatedAt      time.Time
	SubmittedAt    *time.Time
	ResolvedAt     *time.Time
}

type UserStats struct {
	Confirmed, ConfirmedSum, UnpaidAdvances int
	LastConfirmedAt, FirstConfirmedAt       *time.Time
	// HasHistory — есть платежи кроме excludeID.
	HasHistory bool
}
