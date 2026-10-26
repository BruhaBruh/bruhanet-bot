package tgupdate

import "github.com/go-telegram/bot/models"

const (
	TypeMessage       = "message"
	TypePhoto         = "photo"
	TypeEditedMessage = "edited_message"
	TypeCallback      = "callback"
	TypeOther         = "other"
)

func User(u *models.Update) *models.User {
	switch {
	case u == nil:
		return nil
	case u.Message != nil:
		return u.Message.From
	case u.EditedMessage != nil:
		return u.EditedMessage.From
	case u.CallbackQuery != nil:
		return &u.CallbackQuery.From
	}
	return nil
}

func UserID(u *models.Update) int64 {
	if user := User(u); user != nil {
		return user.ID
	}
	return 0
}

func ChatID(u *models.Update) int64 {
	switch {
	case u == nil:
		return 0
	case u.Message != nil:
		return u.Message.Chat.ID
	case u.EditedMessage != nil:
		return u.EditedMessage.Chat.ID
	case u.CallbackQuery != nil:
		m := u.CallbackQuery.Message
		if m.Message != nil {
			return m.Message.Chat.ID
		}
		if m.InaccessibleMessage != nil {
			return m.InaccessibleMessage.Chat.ID
		}
	}
	return 0
}

func Type(u *models.Update) string {
	switch {
	case u == nil:
		return TypeOther
	case u.Message != nil && len(u.Message.Photo) > 0:
		return TypePhoto
	case u.Message != nil:
		return TypeMessage
	case u.EditedMessage != nil:
		return TypeEditedMessage
	case u.CallbackQuery != nil:
		return TypeCallback
	}
	return TypeOther
}
