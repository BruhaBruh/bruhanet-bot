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
	ID             int64         `json:"id"`
	TelegramID     int64         `json:"telegramId"`
	Username       string        `json:"username"`
	Month          int           `json:"month"`
	Price          int           `json:"price"`
	Status         PaymentStatus `json:"status"`
	PrevExpireAt   time.Time     `json:"prevExpireAt"`
	AdvanceGranted bool          `json:"advanceGranted"`
	AdminMessageID int           `json:"adminMessageId"`
	CreatedAt      time.Time     `json:"createdAt"`
	SubmittedAt    *time.Time    `json:"submittedAt"`
	ResolvedAt     *time.Time    `json:"resolvedAt"`
	// PromoID и BasePrice (цена тарифа до скидки) — nil у платежей без промокода.
	PromoID   *int64 `json:"promoId"`
	BasePrice *int   `json:"basePrice"`
}

type UserStats struct {
	Confirmed        int        `json:"confirmed"`
	ConfirmedSum     int        `json:"confirmedSum"`
	UnpaidAdvances   int        `json:"unpaidAdvances"`
	LastConfirmedAt  *time.Time `json:"lastConfirmedAt"`
	FirstConfirmedAt *time.Time `json:"firstConfirmedAt"`
	// HasHistory — есть платежи кроме excludeID.
	HasHistory bool `json:"hasHistory"`
}

// UserSummary — строка списка юзеров в админке.
type UserSummary struct {
	TelegramID   int64     `json:"telegramId"`
	Username     string    `json:"username"`
	CreatedAt    time.Time `json:"createdAt"`
	Confirmed    int       `json:"confirmed"`
	ConfirmedSum int       `json:"confirmedSum"`
}

// PaymentFilter — Status "" = все; From/To — полуинтервал по created_at (нулевое время = без границы).
type PaymentFilter struct {
	Status        PaymentStatus
	From, To      time.Time
	Limit, Offset int
}
