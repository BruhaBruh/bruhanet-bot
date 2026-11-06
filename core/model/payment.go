package model

import "time"

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentConfirmed PaymentStatus = "confirmed"
	PaymentDeclined  PaymentStatus = "declined"
	PaymentCancelled PaymentStatus = "cancelled"
)

// PaymentKind — что оплачено: подписка или докупка устройств.
type PaymentKind string

const (
	KindTariff  PaymentKind = "tariff"
	KindDevices PaymentKind = "devices"
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
	// Kind — tariff|devices. У devices Month = 0, срок подписки платёж не меняет.
	Kind PaymentKind `json:"kind"`
	// ExtraDevices: у devices — сколько устройств куплено; у tariff — доп. устройства юзера после платежа
	// (выбор на момент создания: подтверждение применяет это число, а не пересчитывает).
	ExtraDevices int `json:"extraDevices"`
	// UserExtra — доп. устройства юзера на момент решения (ResolvePayment под блокировкой); только в apply.
	UserExtra int `json:"-"`
	// PhotoFileID — file_id чека; наружу не отдаётся, панель смотрит HasPhoto и GET /api/payments/{id}/photo.
	PhotoFileID string `json:"-"`
	HasPhoto    bool   `json:"hasPhoto"`
	// PromoCode — код promo_id на момент чтения.
	PromoCode *string `json:"promoCode"`
}

type UserStats struct {
	Confirmed        int        `json:"confirmed"`
	ConfirmedSum     int        `json:"confirmedSum"`
	UnpaidAdvances   int        `json:"unpaidAdvances"`
	LastConfirmedAt  *time.Time `json:"lastConfirmedAt"`
	FirstConfirmedAt *time.Time `json:"firstConfirmedAt"`
	// HasHistory — есть платежи кроме excludeID.
	HasHistory bool `json:"hasHistory"`
	// Blocked — юзер заблокирован админом вручную.
	Blocked bool `json:"blocked"`
}

// PaymentFilter — Status "" = все; From/To — полуинтервал по created_at (нулевое время = без границы).
type PaymentFilter struct {
	Status        PaymentStatus
	From, To      time.Time
	Limit, Offset int
}

// ExtraAfter — доп. устройства юзера после подтверждения платежа, если до него было cur.
func (p Payment) ExtraAfter(cur int) int {
	if p.Kind == KindDevices {
		return cur + p.ExtraDevices
	}
	return p.ExtraDevices
}
