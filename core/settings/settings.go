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
	"unicode/utf16"
	"unicode/utf8"

	"github.com/bruhabruh/bruhanet/core/texts"
)

var ErrInvalidSetting = errors.New("invalid setting")

const (
	fallbackReminderTime = "12:00"
	maxSupportURL        = 256 // texts: Max общей переменной supportUrl
	maxVars              = 50
	maxReminderTimes     = 6
)

var (
	clockTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	varName   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)
)

type Settings struct {
	Prices     map[int]int `json:"prices"` // месяцы → ₽
	Phone      string      `json:"phone"`
	Bank       string      `json:"bank"`
	Comments   []string    `json:"comments"` // комментарии к переводу
	SupportURL string      `json:"supportUrl"`

	ReminderTimes    []string `json:"reminderTimes"`    // времена напоминаний по МСК, HH:MM: 1..maxReminderTimes, без повторов, по возрастанию
	AdvanceLimit     int      `json:"advanceLimit"`     // сколько неоплаченных авансов подряд допустимо; 0 — авансов нет
	AdvanceHours     int      `json:"advanceHours"`     // длина аванса
	ReminderWeekDays int      `json:"reminderWeekDays"` // порог «скоро закончится»
	ReminderDayHours int      `json:"reminderDayHours"` // порог «в течение суток»

	DevicesBase int `json:"devicesBase"` // устройств в подписке без доплаты, 1..100
	DevicePrice int `json:"devicePrice"` // ₽ за каждое доп. устройство в месяц; 0 — продажа устройств выключена

	TrialDays    int `json:"trialDays"`    // дней пробного периода; 0 — пробный выключен, 0..30
	TrialDevices int `json:"trialDevices"` // устройств на пробном периоде, 1..100

	ReferralDays int `json:"referralDays"` // дней обоим за первую оплату друга; 0 — рефералка выключена, 0..365

	Vars map[string]string `json:"vars"` // свои переменные текстов: имя → значение
}

// DevicesEnabled — продажа доп. устройств включена.
func (s Settings) DevicesEnabled() bool { return s.DevicePrice > 0 }

// ReferralEnabled — рефералка включена.
func (s Settings) ReferralEnabled() bool { return s.ReferralDays > 0 }

// TrialEnabled — пробный период включён.
func (s Settings) TrialEnabled() bool { return s.TrialDays > 0 }

func Defaults(prices map[int]int, phone, bank, supportURL string) Settings {
	return Settings{
		Prices:     prices,
		Phone:      phone,
		Bank:       bank,
		Comments:   []string{"На кофе", "Перекус", "Одолжил", "Вернул долг"},
		SupportURL: supportURL,

		ReminderTimes:    []string{fallbackReminderTime},
		AdvanceLimit:     3,
		AdvanceHours:     24,
		ReminderWeekDays: 7,
		ReminderDayHours: 24,

		DevicesBase: 5,
		DevicePrice: 0,

		TrialDays:    0,
		TrialDevices: 2,

		ReferralDays: 0,

		Vars: map[string]string{},
	}
}

// TextVars — общие переменные текстов, которые берутся из настроек (новая карта: снимок провайдера не трогается).
func (s Settings) TextVars() map[string]string {
	out := make(map[string]string, len(s.Vars)+3)
	maps.Copy(out, s.Vars)
	out["supportUrl"], out["phone"], out["bank"] = s.SupportURL, s.Phone, s.Bank
	return out
}

// WithCron берёт времена напоминаний по умолчанию из NOTIFICATION_CRON: «M h1,h2,… * * *» с одной минутой.
// Другое выражение — остаётся запасное время и непустое предупреждение для лога.
func (s Settings) WithCron(spec string) (Settings, string) {
	fallback := func() (Settings, string) {
		return s, "NOTIFICATION_CRON is not a daily expression with fixed minute, using " + strings.Join(s.ReminderTimes, ",")
	}
	f := strings.Fields(spec)
	if len(f) != 5 || f[2] != "*" || f[3] != "*" || f[4] != "*" {
		return fallback()
	}
	minute, err := strconv.Atoi(f[0])
	if err != nil || minute < 0 || minute > 59 {
		return fallback()
	}
	var times []string
	for h := range strings.SplitSeq(f[1], ",") {
		hour, err := strconv.Atoi(h)
		if err != nil || hour < 0 || hour > 23 {
			return fallback()
		}
		times = append(times, fmt.Sprintf("%02d:%02d", hour, minute))
	}
	slices.Sort(times)
	times = slices.Compact(times)
	if len(times) > maxReminderTimes {
		return fallback()
	}
	s.ReminderTimes = times
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
	// прежний ключ — одно время; reminderTimes, если есть, главнее
	apply("reminderTime", func(b []byte) error {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		def.ReminderTimes = []string{v}
		return nil
	})
	apply("reminderTimes", func(b []byte) error {
		var v []string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		def.ReminderTimes = v
		return nil
	})
	num := func(dst *int) func([]byte) error {
		return func(b []byte) error { return json.Unmarshal(b, dst) }
	}
	apply("advanceLimit", num(&def.AdvanceLimit))
	apply("advanceHours", num(&def.AdvanceHours))
	apply("reminderWeekDays", num(&def.ReminderWeekDays))
	apply("reminderDayHours", num(&def.ReminderDayHours))
	apply("devicesBase", num(&def.DevicesBase))
	apply("devicePrice", num(&def.DevicePrice))
	apply("trialDays", num(&def.TrialDays))
	apply("trialDevices", num(&def.TrialDevices))
	apply("referralDays", num(&def.ReferralDays))
	apply("vars", func(b []byte) error {
		var v map[string]string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		def.Vars = v
		return nil
	})
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
	} else if utf8.RuneCountInString(s.SupportURL) > maxSupportURL {
		p = append(p, fmt.Sprintf("ссылка поддержки должна быть не длиннее %d символов", maxSupportURL))
	}
	p = append(p, s.reminderTimeProblems()...)
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
	if s.DevicesBase < 1 || s.DevicesBase > 100 {
		p = append(p, "устройств в базе должно быть от 1 до 100")
	}
	if s.DevicePrice < 0 || s.DevicePrice > 100_000 {
		p = append(p, "цена доп. устройства должна быть от 0 до 100 000 ₽ в месяц")
	}
	if s.TrialDays < 0 || s.TrialDays > 30 {
		p = append(p, "дней пробного периода должно быть от 0 до 30")
	}
	if s.TrialDevices < 1 || s.TrialDevices > 100 {
		p = append(p, "устройств на пробном периоде должно быть от 1 до 100")
	}
	if s.ReferralDays < 0 || s.ReferralDays > 365 {
		p = append(p, "дней за приглашённого друга должно быть от 0 до 365")
	}
	if len(s.Vars) > maxVars {
		p = append(p, fmt.Sprintf("своих переменных должно быть не больше %d", maxVars))
	}
	for _, name := range slices.Sorted(maps.Keys(s.Vars)) {
		switch {
		case !varName.MatchString(name):
			p = append(p, fmt.Sprintf("имя переменной «%s»: латиница, цифры и _, первая — буква", name))
		case texts.Reserved(name):
			p = append(p, fmt.Sprintf("имя переменной «%s» занято системной", name))
		}
		if strings.TrimSpace(s.Vars[name]) == "" {
			p = append(p, fmt.Sprintf("значение переменной «%s» не может быть пустым", name))
		}
	}
	return problemsErr(p)
}

// ValidateForSave — Validate плюс лимит длины значений своих переменных. При загрузке лимит не проверяется:
// значения, сохранённые до его появления, не должны сбрасывать бота на env-дефолты.
func (s Settings) ValidateForSave() error {
	var p []string
	if ve, ok := errors.AsType[*ValidationError](s.Validate()); ok {
		p = ve.Items
	}
	return problemsErr(append(p, s.LongVarProblems()...))
}

// LongVarProblems — свои переменные, значение которых длиннее texts.MaxCustomVar (UTF-16).
func (s Settings) LongVarProblems() []string {
	var p []string
	for _, name := range slices.Sorted(maps.Keys(s.Vars)) {
		if len(utf16.Encode([]rune(s.Vars[name]))) > texts.MaxCustomVar {
			p = append(p, fmt.Sprintf("Переменная %s длиннее %d символов: сократите", name, texts.MaxCustomVar))
		}
	}
	return p
}

func (s Settings) reminderTimeProblems() []string {
	var p []string
	if n := len(s.ReminderTimes); n < 1 || n > maxReminderTimes {
		p = append(p, fmt.Sprintf("времён напоминаний должно быть от 1 до %d", maxReminderTimes))
	}
	seen := map[string]bool{}
	for _, t := range s.ReminderTimes {
		switch {
		case !clockTime.MatchString(t):
			p = append(p, fmt.Sprintf("время напоминаний «%s» должно быть в формате ЧЧ:ММ", t))
		case seen[t]:
			p = append(p, fmt.Sprintf("время напоминаний %s повторяется", t))
		}
		seen[t] = true
	}
	return p
}

// SortedReminderTimes — времена по возрастанию (копия): порядок в БД не важен.
func (s Settings) SortedReminderTimes() []string {
	return slices.Sorted(slices.Values(s.ReminderTimes))
}
