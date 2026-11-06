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

// PanelUser — юзер панели Remnawave с Telegram ID.
type PanelUser struct {
	TelegramID int64
	Username   string // "" — нет telegram username
	Status     string
	ExpireAt   time.Time
	CreatedAt  time.Time
}

// PanelSync — итог синхронизации: Total — юзеров в панели, Updated — записанных строк.
type PanelSync struct {
	Total      int       `json:"total"`
	Updated    int       `json:"updated"`
	FinishedAt time.Time `json:"finishedAt"`
}
