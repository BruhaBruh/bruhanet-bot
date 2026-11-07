// Package broadcast — правила рассылки из админки: статусы, аудитории, кнопки под сообщением.
package broadcast

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/core/usererr"
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
	MaxButtons    = 6
	MaxRowButtons = 2
	MaxButtonText = 64 // UTF-16, как у кнопок бота
	MaxButtonURL  = 512
	MinDays       = 1
	MaxDays       = 60
)

var (
	ErrAudience = errors.New("invalid audience")
	ErrButtons  = errors.New("invalid buttons")
)

// Button — url-кнопка под сообщением рассылки; Style — значения texts.Styles.
type Button struct {
	Text  string `json:"text"`
	URL   string `json:"url"`
	Style string `json:"style"`
	// Inline: кнопка в одной строке с предыдущей; нет — своя строка (так хранятся старые рассылки).
	Inline bool `json:"inline,omitempty"`
}

// ValidateAudience: expiring требует days 1..60, остальные days не принимают (nil).
func ValidateAudience(audience string, days *int) error {
	if !slices.Contains(Audiences, audience) {
		return errors.Join(ErrAudience, usererr.Errorf("unknown audience %q", "неизвестная аудитория «%s»", audience))
	}
	switch {
	case audience != AudienceExpiring && days != nil:
		return errors.Join(ErrAudience, usererr.Errorf("days are only valid for the expiring audience", "число дней нужно только для «подписка истекает»"))
	case audience == AudienceExpiring && (days == nil || *days < MinDays || *days > MaxDays):
		return errors.Join(ErrAudience, usererr.Errorf("expiring audience needs days in %d..%d", "«подписка истекает» — от %d до %d дней", MinDays, MaxDays))
	}
	return nil
}

// ValidateButtons — до 6 кнопок, в строке не больше 2 (первая не inline): текст 1..64 простым текстом (без переменных), https-ссылка, стиль из texts.Styles.
// javascript:, data:, tg: и http: отклоняются: разрешён только https с хостом.
func ValidateButtons(bs []Button) error {
	if len(bs) > MaxButtons {
		return errors.Join(ErrButtons, usererr.Errorf("at most %d buttons", "кнопок может быть не больше %d", MaxButtons))
	}
	inRow := 0
	for i, b := range bs {
		n := i + 1
		if b.Inline {
			inRow++
			if i == 0 || inRow > MaxRowButtons {
				return errors.Join(ErrButtons, usererr.Errorf("button %d: at most %d buttons in a row, the first cannot share a row", "кнопка %d: в строке не больше %d кнопок, первая кнопка всегда в своей строке", n, MaxRowButtons))
			}
		} else {
			inRow = 1
		}
		if t := strings.TrimSpace(b.Text); t == "" || t != b.Text || len(utf16.Encode([]rune(t))) > MaxButtonText {
			return errors.Join(ErrButtons, usererr.Errorf("button %d: text must be 1..%d characters without surrounding spaces", "кнопка %d: текст от 1 до %d символов, без пробелов по краям", n, MaxButtonText))
		}
		if strings.Contains(b.Text, "{{") {
			return errors.Join(ErrButtons, usererr.Errorf("button %d: variables are not allowed in button text", "кнопка %d: в тексте кнопки нельзя использовать переменные", n))
		}
		if !slices.Contains(texts.Styles, b.Style) {
			return errors.Join(ErrButtons, usererr.Errorf("button %d: unknown style", "кнопка %d: неизвестный стиль", n))
		}
		if !validURL(b.URL) {
			return errors.Join(ErrButtons, usererr.Errorf("button %d: url must be https and at most %d characters", "кнопка %d: ссылка должна начинаться с https:// и быть не длиннее %d символов", n, MaxButtonURL))
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

// Rows группирует кнопки в строки: Inline — в строку предыдущей (если там меньше 2), иначе новая строка.
func Rows(bs []Button) [][]Button {
	var rows [][]Button
	for _, b := range bs {
		if n := len(rows); b.Inline && n > 0 && len(rows[n-1]) < MaxRowButtons {
			rows[n-1] = append(rows[n-1], b)
			continue
		}
		rows = append(rows, []Button{b})
	}
	return rows
}

// Keyboard — клавиатура под сообщением по строкам Rows; нет кнопок — nil.
func Keyboard(bs []Button) *models.InlineKeyboardMarkup {
	if len(bs) == 0 {
		return nil
	}
	var rows [][]models.InlineKeyboardButton
	for _, r := range Rows(bs) {
		row := make([]models.InlineKeyboardButton, len(r))
		for i, b := range r {
			row[i] = models.InlineKeyboardButton{Text: b.Text, URL: b.URL, Style: b.Style}
		}
		rows = append(rows, row)
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
