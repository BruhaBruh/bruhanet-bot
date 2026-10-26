package model

import "time"

type Payment struct {
	TelegramID           int64     `json:"telegramId"`
	Username             string    `json:"username"`
	Month                int       `json:"month"`
	Price                int       `json:"price"`
	SubscriptionExpireAt time.Time `json:"subscriptionExpireAt"`
}
