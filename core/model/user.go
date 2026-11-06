package model

import "time"

// UserSeen — данные юзера из апдейта Telegram.
type UserSeen struct {
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
	At         time.Time
}
