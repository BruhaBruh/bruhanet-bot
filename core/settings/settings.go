// Package settings — настройки бота, редактируемые из админки.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalidSetting = errors.New("некорректная настройка")

const fallbackReminderTime = "12:00"

var clockTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

type Settings struct {
	Prices     map[int]int `json:"prices"` // месяцы → ₽
	Phone      string      `json:"phone"`
	Bank       string      `json:"bank"`
	Comments   []string    `json:"comments"` // комментарии к переводу
	SupportURL string      `json:"supportUrl"`

	ReminderTime     string `json:"reminderTime"`     // время напоминаний по МСК, HH:MM
	AdvanceLimit     int    `json:"advanceLimit"`     // сколько неоплаченных авансов подряд допустимо; 0 — авансов нет
	AdvanceHours     int    `json:"advanceHours"`     // длина аванса
	ReminderWeekDays int    `json:"reminderWeekDays"` // порог «скоро закончится»
	ReminderDayHours int    `json:"reminderDayHours"` // порог «в течение суток»
}

func Defaults(prices map[int]int, phone, bank, supportURL string) Settings {
	return Settings{
		Prices:     prices,
		Phone:      phone,
		Bank:       bank,
		Comments:   []string{"На кофе", "Перекус", "Одолжил", "Вернул долг"},
		SupportURL: supportURL,

		ReminderTime:     fallbackReminderTime,
		AdvanceLimit:     3,
		AdvanceHours:     24,
		ReminderWeekDays: 7,
		ReminderDayHours: 24,
	}
}

// WithCron берёт время напоминаний по умолчанию из NOTIFICATION_CRON.
// Выражение не вида «M H * * *» — остаётся запасное время и непустое предупреждение для лога.
func (s Settings) WithCron(spec string) (Settings, string) {
	f := strings.Fields(spec)
	if len(f) != 5 || f[2] != "*" || f[3] != "*" || f[4] != "*" {
		return s, "NOTIFICATION_CRON is not a daily expression, using " + s.ReminderTime
	}
	hours := strings.Split(f[1], ",")
	minute, merr := strconv.Atoi(f[0])
	hour, herr := strconv.Atoi(hours[0])
	if merr != nil || herr != nil || minute < 0 || minute > 59 || hour < 0 || hour > 23 {
		return s, "NOTIFICATION_CRON is not a daily expression, using " + s.ReminderTime
	}
	s.ReminderTime = fmt.Sprintf("%02d:%02d", hour, minute)
	if len(hours) > 1 {
		return s, "NOTIFICATION_CRON has several hours, only the first is used: " + s.ReminderTime
	}
	return s, ""
}

// ValidationError — список проблем настроек; Problems отдаёт их без служебного префикса.
type ValidationError struct{ Items []string }

func (e *ValidationError) Error() string {
	return ErrInvalidSetting.Error() + "\n" + strings.Join(e.Items, "; ")
}

func (e *ValidationError) Is(target error) bool { return target == ErrInvalidSetting }

// Problems — человеческие тексты проблем из ошибки Merge/Validate; nil, если err не про настройки.
func Problems(err error) []string {
	if ve, ok := errors.AsType[*ValidationError](err); ok {
		return ve.Items
	}
	return nil
}

func problemsErr(p []string) error {
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Items: p}
}

// Merge накладывает raw (key → JSON) на def; каждый ключ независим.
// Неизвестные ключи игнорируются, битое значение (в т.ч. null) даёт ошибку, остальные применяются.
func Merge(def Settings, raw map[string][]byte) (Settings, error) {
	var p []string
	// ключи в фиксированном порядке — сообщения детерминированы
	apply := func(key string, set func(b []byte) error) {
		b, ok := raw[key]
		if !ok {
			return
		}
		if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
			p = append(p, fmt.Sprintf("ключ %q: значение не может быть null", key))
			return
		}
		if err := set(b); err != nil {
			p = append(p, fmt.Sprintf("ключ %q: %v", key, err))
		}
	}
	// декодируем во временное значение, чтобы битый JSON не испортил поле
	str := func(dst *string) func([]byte) error {
		return func(b []byte) error {
			var v string
			if err := json.Unmarshal(b, &v); err != nil {
				return err
			}
			*dst = v
			return nil
		}
	}
	apply("prices", func(b []byte) error {
		var v map[int]int
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		def.Prices = v
		return nil
	})
	apply("phone", str(&def.Phone))
	apply("bank", str(&def.Bank))
	apply("comments", func(b []byte) error {
		var v []string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		def.Comments = v
		return nil
	})
	apply("supportUrl", str(&def.SupportURL))
	apply("reminderTime", str(&def.ReminderTime))
	num := func(dst *int) func([]byte) error {
		return func(b []byte) error { return json.Unmarshal(b, dst) }
	}
	apply("advanceLimit", num(&def.AdvanceLimit))
	apply("advanceHours", num(&def.AdvanceHours))
	apply("reminderWeekDays", num(&def.ReminderWeekDays))
	apply("reminderDayHours", num(&def.ReminderDayHours))
	return def, problemsErr(p)
}

// Validate проверяет границы; сообщения на русском — их видит админ (Problems).
func (s Settings) Validate() error {
	var p []string
	if len(s.Prices) == 0 {
		p = append(p, "нужен хотя бы один тариф")
	}
	for _, m := range slices.Sorted(maps.Keys(s.Prices)) {
		price := s.Prices[m]
		if m < 1 || m > 120 {
			p = append(p, fmt.Sprintf("срок тарифа должен быть от 1 до 120 месяцев, получено %d", m))
		}
		if price < 1 || price > 1_000_000 {
			p = append(p, fmt.Sprintf("цена тарифа на %d мес. должна быть от 1 до 1 000 000 ₽", m))
		}
	}
	for _, f := range []struct{ name, v string }{{"телефон", s.Phone}, {"банк", s.Bank}} {
		if n := utf8.RuneCountInString(strings.TrimSpace(f.v)); n < 1 || n > 64 {
			p = append(p, f.name+" должен быть от 1 до 64 символов")
		}
	}
	if n := len(s.Comments); n < 1 || n > 10 {
		p = append(p, "комментариев должно быть от 1 до 10")
	}
	for i, c := range s.Comments {
		if n := utf8.RuneCountInString(strings.TrimSpace(c)); n < 1 || n > 64 {
			p = append(p, fmt.Sprintf("комментарий %d («%s») должен быть от 1 до 64 символов", i+1, c))
		}
	}
	if u, err := url.Parse(s.SupportURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p = append(p, "ссылка поддержки должна начинаться с http:// или https://")
	}
	if !clockTime.MatchString(s.ReminderTime) {
		p = append(p, "время напоминаний должно быть в формате ЧЧ:ММ")
	}
	if s.AdvanceLimit < 0 || s.AdvanceLimit > 100 {
		p = append(p, "лимит авансов должен быть от 0 до 100")
	}
	if s.AdvanceHours < 1 || s.AdvanceHours > 720 {
		p = append(p, "длина аванса должна быть от 1 до 720 часов")
	}
	if s.ReminderWeekDays < 1 || s.ReminderWeekDays > 60 {
		p = append(p, "первое напоминание должно быть за 1–60 дней")
	}
	if s.ReminderDayHours < 1 || s.ReminderDayHours >= s.ReminderWeekDays*24 {
		p = append(p, "второе напоминание должно быть позже первого и не меньше чем за 1 час")
	}
	return problemsErr(p)
}
