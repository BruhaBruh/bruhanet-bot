// Package broadcast — правила рассылки из админки: статусы, аудитории, кнопки под сообщением.
package broadcast

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/texts"
)

const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusSending   = "sending"
	StatusSent      = "sent"
	StatusCancelled = "cancelled"
)

// Statuses — все статусы рассылки.
var Statuses = []string{StatusDraft, StatusScheduled, StatusSending, StatusSent, StatusCancelled}

const (
	AudienceAll            = "all"
	AudienceActive         = "active"
	AudienceNoSubscription = "no_subscription"
	AudienceExpiring       = "expiring"
	AudienceTrial          = "trial"
)

// Audiences — аудитории рассылки.
var Audiences = []string{AudienceAll, AudienceActive, AudienceNoSubscription, AudienceExpiring, AudienceTrial}

const (
	MaxButtons    = 3
	MaxButtonText = 64 // UTF-16, как у кнопок бота
	MaxButtonURL  = 512
	MinDays       = 1
	MaxDays       = 60
)

var (
	ErrAudience = errors.New("некорректная аудитория")
	ErrButtons  = errors.New("некорректные кнопки")
)

// Button — url-кнопка под сообщением рассылки; Style — значения texts.Styles.
type Button struct {
	Text  string `json:"text"`
	URL   string `json:"url"`
	Style string `json:"style"`
}

// ValidateAudience: expiring требует days 1..60, остальные days не принимают (nil).
func ValidateAudience(audience string, days *int) error {
	if !slices.Contains(Audiences, audience) {
		return errors.Join(ErrAudience, fmt.Errorf("неизвестная аудитория «%s»", audience))
	}
	switch {
	case audience != AudienceExpiring && days != nil:
		return errors.Join(ErrAudience, errors.New("число дней нужно только для «подписка истекает»"))
	case audience == AudienceExpiring && (days == nil || *days < MinDays || *days > MaxDays):
		return errors.Join(ErrAudience, fmt.Errorf("«подписка истекает» — от %d до %d дней", MinDays, MaxDays))
	}
	return nil
}

// ValidateButtons — до 3 кнопок: текст 1..64 простым текстом (без переменных), https-ссылка, стиль из texts.Styles.
// javascript:, data:, tg: и http: отклоняются: разрешён только https с хостом.
func ValidateButtons(bs []Button) error {
	if len(bs) > MaxButtons {
		return errors.Join(ErrButtons, fmt.Errorf("кнопок может быть не больше %d", MaxButtons))
	}
	for i, b := range bs {
		n := i + 1
		if t := strings.TrimSpace(b.Text); t == "" || t != b.Text || len(utf16.Encode([]rune(t))) > MaxButtonText {
			return errors.Join(ErrButtons, fmt.Errorf("кнопка %d: текст от 1 до %d символов, без пробелов по краям", n, MaxButtonText))
		}
		if strings.Contains(b.Text, "{{") {
			return errors.Join(ErrButtons, fmt.Errorf("кнопка %d: в тексте кнопки нельзя использовать переменные", n))
		}
		if !slices.Contains(texts.Styles, b.Style) {
			return errors.Join(ErrButtons, fmt.Errorf("кнопка %d: неизвестный стиль", n))
		}
		if !validURL(b.URL) {
			return errors.Join(ErrButtons, fmt.Errorf("кнопка %d: ссылка должна начинаться с https:// и быть не длиннее %d символов", n, MaxButtonURL))
		}
	}
	return nil
}

func validURL(raw string) bool {
	if raw == "" || len(raw) > MaxButtonURL || strings.ContainsFunc(raw, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

// Keyboard — клавиатура под сообщением, по кнопке в строке; нет кнопок — nil.
func Keyboard(bs []Button) *models.InlineKeyboardMarkup {
	if len(bs) == 0 {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, len(bs))
	for i, b := range bs {
		rows[i] = []models.InlineKeyboardButton{{Text: b.Text, URL: b.URL, Style: b.Style}}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
