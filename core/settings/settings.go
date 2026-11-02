// Package settings — настройки бота, редактируемые из админки.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// Merge накладывает raw (key → JSON) на def; каждый ключ независим.
// Неизвестные ключи игнорируются, битое значение даёт ошибку, остальные применяются.
func Merge(def Settings, raw map[string][]byte) (Settings, error) {
	fields := map[string]any{
		"prices":     &def.Prices,
		"phone":      &def.Phone,
		"bank":       &def.Bank,
		"comments":   &def.Comments,
		"supportUrl": &def.SupportURL,
	}
	var errs []error
	for k, dst := range fields {
		b, ok := raw[k]
		if !ok {
			continue
		}
		// декодируем во временное значение, чтобы битый JSON не испортил поле
		switch d := dst.(type) {
		case *map[int]int:
			var v map[int]int
			if err := json.Unmarshal(b, &v); err != nil {
				errs = append(errs, bad(k, err))
				continue
			}
			*d = v
		case *[]string:
			var v []string
			if err := json.Unmarshal(b, &v); err != nil {
				errs = append(errs, bad(k, err))
				continue
			}
			*d = v
		case *string:
			var v string
			if err := json.Unmarshal(b, &v); err != nil {
				errs = append(errs, bad(k, err))
				continue
			}
			*d = v
		}
	}
	return def, errors.Join(errs...)
}

func bad(key string, err error) error {
	return errors.Join(ErrInvalidSetting, fmt.Errorf("ключ %q: %w", key, err))
}

// Validate проверяет границы; сообщения на русском — их видит админ.
func (s Settings) Validate() error {
	var p []string
	if len(s.Prices) == 0 {
		p = append(p, "нужен хотя бы один тариф")
	}
	for m, price := range s.Prices {
		if m < 1 || m > 120 {
			p = append(p, fmt.Sprintf("срок тарифа должен быть от 1 до 120 месяцев, получено %d", m))
		}
		if price < 1 || price > 1_000_000 {
			p = append(p, fmt.Sprintf("цена тарифа на %d мес. должна быть от 1 до 1 000 000 ₽", m))
		}
	}
	if strings.TrimSpace(s.Phone) == "" {
		p = append(p, "телефон не может быть пустым")
	}
	if strings.TrimSpace(s.Bank) == "" {
		p = append(p, "банк не может быть пустым")
	}
	if n := len(s.Comments); n < 1 || n > 10 {
		p = append(p, "комментариев должно быть от 1 до 10")
	}
	for _, c := range s.Comments {
		if n := utf8.RuneCountInString(strings.TrimSpace(c)); n < 1 || n > 64 {
			p = append(p, "каждый комментарий должен быть от 1 до 64 символов")
			break
		}
	}
	if u, err := url.Parse(s.SupportURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p = append(p, "ссылка поддержки должна начинаться с http:// или https://")
	}
	if len(p) == 0 {
		return nil
	}
	return errors.Join(ErrInvalidSetting, errors.New(strings.Join(p, "; ")))
}
