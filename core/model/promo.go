package model

import "time"

type PromoKind string

const (
	PromoPercent PromoKind = "percent"
	PromoRub     PromoKind = "rub"
	PromoDays    PromoKind = "days"
)

// Promo — промокод. Nil-ограничение = без ограничения; TelegramID — персональный код.
type Promo struct {
	ID             int64      `json:"id"`
	Code           string     `json:"code"`
	Kind           PromoKind  `json:"kind"`
	Value          int        `json:"value"`
	MaxActivations *int       `json:"maxActivations"`
	StartsAt       *time.Time `json:"startsAt"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	OnlyNew        bool       `json:"onlyNew"`
	TelegramID     *int64     `json:"telegramId"`
	Disabled       bool       `json:"disabled"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// Значения PromoFilter; пустая строка — без фильтра.
const (
	PromoStatusActive   = "active"
	PromoStatusExpired  = "expired"
	PromoStatusDisabled = "disabled"

	PromoTargetAll  = "all"
	PromoTargetNew  = "new"
	PromoTargetUser = "user"
)

// PromoFilter — фильтры списка кодов: Status active|expired|disabled, Kind — PromoKind, Target all|new|user.
type PromoFilter struct{ Status, Kind, Target string }

// PromoWithUsage — промокод со счётчиком активаций.
type PromoWithUsage struct {
	Promo
	Used int `json:"used"`
}

// PromoActivation — активация кода юзером; PaymentID nil у кода на дни.
type PromoActivation struct {
	ID         int64     `json:"id"`
	PromoID    int64     `json:"promoId"`
	TelegramID int64     `json:"telegramId"`
	Username   string    `json:"username"`
	PaymentID  *int64    `json:"paymentId"`
	CreatedAt  time.Time `json:"createdAt"`
}
