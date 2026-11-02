// Package settings — настройки бота, редактируемые из админки.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

var ErrInvalidSetting = errors.New("некорректная настройка")

type Settings struct {
	Prices     map[int]int `json:"prices"` // месяцы → ₽
	Phone      string      `json:"phone"`
	Bank       string      `json:"bank"`
	Comments   []string    `json:"comments"` // комментарии к переводу
	SupportURL string      `json:"supportUrl"`
}

func Defaults(prices map[int]int, phone, bank, supportURL string) Settings {
	return Settings{
		Prices:     prices,
		Phone:      phone,
		Bank:       bank,
		Comments:   []string{"На кофе", "Перекус", "Одолжил", "Вернул долг"},
		SupportURL: supportURL,
	}
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
	return problemsErr(p)
}
