package model

import (
	"errors"
	"time"
)

// ErrGiftUnavailable — подарок нельзя активировать. Причину (нет такого кода, уже использован, отменён, не оплачен,
// юзер заблокирован) не раскрываем: ответ один на все случаи.
var ErrGiftUnavailable = errors.New("gift is not available")

type GiftStatus string

const (
	GiftPending   GiftStatus = "pending"
	GiftReady     GiftStatus = "ready"
	GiftRedeemed  GiftStatus = "redeemed"
	GiftCancelled GiftStatus = "cancelled"
)

// MaxGiftMonths — потолок срока подарка (месяцев).
const MaxGiftMonths = 120

// Gift — подарочная подписка. Code у pending-подарка наружу не отдаётся (его знает только БД до подтверждения платежа).
type Gift struct {
	ID        int64      `json:"id"`
	Code      string     `json:"code"`
	Months    int        `json:"months"`
	Status    GiftStatus `json:"status"`
	BuyerID   *int64     `json:"buyerId"`
	BuyerName string     `json:"buyerUsername"`
	// BuyerFullName и RedeemedFullName — имя и фамилия из users; пусто, если неизвестны.
	BuyerFullName string `json:"buyerName"`
	PaymentID     *int64 `json:"paymentId"`
	ByAdmin       bool   `json:"byAdmin"`
	RedeemedBy    *int64 `json:"redeemedBy"`
	// RedeemedName — username активировавшего; пусто, если его нет.
	RedeemedName     string     `json:"redeemedUsername"`
	RedeemedFullName string     `json:"redeemedName"`
	RedeemedAt       *time.Time `json:"redeemedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
}

// GiftFilter — Status "" = все; Limit/Offset — страница.
type GiftFilter struct {
	Status        GiftStatus
	Limit, Offset int
}
