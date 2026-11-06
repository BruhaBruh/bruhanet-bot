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

// UserContact — данные юзера для переменных текстов; нет юзера или значения — пустые.
type UserContact struct {
	FirstName string
	Username  string
	ExpireAt  *time.Time
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

// UserSummary — строка списка юзеров в админке; nil — данных ещё нет (не синхронизирован, не заходил).
type UserSummary struct {
	TelegramID         int64      `json:"telegramId"`
	Username           string     `json:"username"`
	Name               string     `json:"name"` // имя и фамилия из Telegram
	CreatedAt          time.Time  `json:"createdAt"`
	Confirmed          int        `json:"confirmed"`
	ConfirmedSum       int        `json:"confirmedSum"`
	ExpireAt           *time.Time `json:"expireAt"`
	PanelStatus        *string    `json:"panelStatus"`
	BlockedBotAt       *time.Time `json:"blockedBotAt"`
	BlockedAt          *time.Time `json:"blockedAt"`    // ручная блокировка админом: без авансов и промокодов
	TariffMonths       *int       `json:"tariffMonths"` // последний подтверждённый платёж
	LastSeenAt         *time.Time `json:"lastSeenAt"`
	BroadcastsReceived int        `json:"broadcastsReceived"`
}

// UserTab — вкладка списка юзеров.
type UserTab string

const (
	UserTabAll        UserTab = ""
	UserTabActive     UserTab = "active"
	UserTabTrial      UserTab = "trial"
	UserTabBlockedBot UserTab = "blocked_bot"
	UserTabBlocked    UserTab = "blocked" // заблокированные админом вручную
)

// UserFilter — From/To: полуинтервал по дате регистрации (нулевое время = без границы); Now — для вкладки «Активные».
type UserFilter struct {
	Q                 string
	Tab               UserTab
	HasPayments       bool
	HasUnpaidAdvances bool
	From, To          time.Time
	Now               time.Time
	Limit, Offset     int
}

// UserPromo — применённый юзером промокод.
type UserPromo struct {
	Code        string    `json:"code"`
	Kind        PromoKind `json:"kind"`
	Value       int       `json:"value"`
	ActivatedAt time.Time `json:"activatedAt"`
}

// Grant — дни, выданные админом.
type Grant struct {
	ID        int64     `json:"id"`
	Days      int       `json:"days"`
	CreatedAt time.Time `json:"createdAt"`
}
