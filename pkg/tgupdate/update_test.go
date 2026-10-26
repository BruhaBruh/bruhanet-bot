package tgupdate_test

import (
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

func TestUpdateFields(t *testing.T) {
	msg := &models.Update{Message: &models.Message{
		From: &models.User{ID: 1}, Chat: models.Chat{ID: 10},
	}}
	photo := &models.Update{Message: &models.Message{
		From: &models.User{ID: 2}, Chat: models.Chat{ID: 20}, Photo: []models.PhotoSize{{FileID: "f"}},
	}}
	edited := &models.Update{EditedMessage: &models.Message{
		From: &models.User{ID: 3}, Chat: models.Chat{ID: 30},
	}}
	callback := &models.Update{CallbackQuery: &models.CallbackQuery{
		From: models.User{ID: 4},
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{Chat: models.Chat{ID: 40}},
		},
	}}
	inaccessible := &models.Update{CallbackQuery: &models.CallbackQuery{
		From: models.User{ID: 5},
		Message: models.MaybeInaccessibleMessage{
			Type:                models.MaybeInaccessibleMessageTypeInaccessibleMessage,
			InaccessibleMessage: &models.InaccessibleMessage{Chat: models.Chat{ID: 50}},
		},
	}}
	other := &models.Update{}

	tests := []struct {
		name   string
		update *models.Update
		userID int64
		chatID int64
		typ    string
	}{
		{"message", msg, 1, 10, tgupdate.TypeMessage},
		{"photo", photo, 2, 20, tgupdate.TypePhoto},
		{"edited", edited, 3, 30, tgupdate.TypeEditedMessage},
		{"callback", callback, 4, 40, tgupdate.TypeCallback},
		{"inaccessible", inaccessible, 5, 50, tgupdate.TypeCallback},
		{"other", other, 0, 0, tgupdate.TypeOther},
		{"nil", nil, 0, 0, tgupdate.TypeOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tgupdate.UserID(tt.update); got != tt.userID {
				t.Errorf("UserID = %d, want %d", got, tt.userID)
			}
			if got := tgupdate.ChatID(tt.update); got != tt.chatID {
				t.Errorf("ChatID = %d, want %d", got, tt.chatID)
			}
			if got := tgupdate.Type(tt.update); got != tt.typ {
				t.Errorf("Type = %q, want %q", got, tt.typ)
			}
		})
	}
}

func TestUserNilFrom(t *testing.T) {
	u := &models.Update{Message: &models.Message{Chat: models.Chat{ID: 1}}}
	if tgupdate.User(u) != nil {
		t.Fatal("ожидался nil для сообщения без From")
	}
}
