package model

// Broadcast — сообщение админа, которое копируется получателям рассылки.
type Broadcast struct {
	ChatID    int64 `json:"chatId"`
	MessageID int   `json:"messageId"`
}
