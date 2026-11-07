// Package promo — правила промокодов: формат кода, проверка применимости, расчёт цены.
package promo

import (
	"crypto/rand"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/usererr"
)

type Kind = model.PromoKind

const (
	KindPercent = model.PromoPercent
	KindRub     = model.PromoRub
	KindDays    = model.PromoDays
)

// Place — где вводится код: скидки — при оплате, дни — из меню.
type Place int

const (
	PlacePayment Place = iota + 1
	PlaceMenu
)

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 32 символа: байт % 32 без перекоса

var codeRe = regexp.MustCompile(`^[A-Z0-9_-]{3,32}$`)

// Normalize приводит ввод к виду хранения: trim + верхний регистр.
func Normalize(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// Generate — случайный код из 8 символов без похожих I/O/0/1.
func Generate() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read не возвращает ошибок
	for i := range b {
		b[i] = alphabet[b[i]%byte(len(alphabet))]
	}
	return string(b)
}

// invalid — отказ ValidateNew: en уходит в логи, ru показывает панель.
func invalid(en, ru string) error { return errors.Join(ErrInvalid, &usererr.Error{En: en, Ru: ru}) }

// ValidateNew проверяет код (после Normalize), тип, значение и ограничения нового промокода.
func ValidateNew(p model.Promo) error {
	if !codeRe.MatchString(Normalize(p.Code)) {
		return invalid("bad code format", "код: 3–32 символа A-Z, 0-9, _ или -")
	}
	var maxValue int
	switch p.Kind {
	case KindPercent:
		maxValue = 100
	case KindRub:
		maxValue = 1_000_000
	case KindDays:
		maxValue = 3650
	default:
		return invalid("unknown kind", "тип: percent, rub или days")
	}
	if p.Value < 1 || p.Value > maxValue {
		return invalid("value out of range", "значение вне диапазона 1.."+strconv.Itoa(maxValue))
	}
	if p.MaxActivations != nil && *p.MaxActivations < 1 {
		return invalid("max activations below 1", "максимум активаций — не меньше 1")
	}
	if p.StartsAt != nil && p.ExpiresAt != nil && !p.StartsAt.Before(*p.ExpiresAt) {
		return invalid("starts_at is not before expires_at", "«Действует с» позже даты окончания")
	}
	if p.TelegramID != nil && *p.TelegramID <= 0 {
		return invalid("bad telegram id", "некорректный telegram id")
	}
	return nil
}

// Check возвращает первую причину, по которой юзер не может применить код здесь и сейчас.
// ErrNotFound вызывающий отдаёт сам (кода нет в базе).
func Check(p model.Promo, used int, telegramID int64, hasConfirmed, alreadyActivated bool, place Place, now time.Time) error {
	switch {
	case p.Disabled || p.Deleted:
		return ErrDisabled
	case p.StartsAt != nil && now.Before(*p.StartsAt):
		return ErrNotStarted
	case p.ExpiresAt != nil && !now.Before(*p.ExpiresAt):
		return ErrExpired
	case p.MaxActivations != nil && used >= *p.MaxActivations:
		return ErrExhausted
	case p.TelegramID != nil && *p.TelegramID != telegramID:
		return ErrNotYours
	case p.OnlyNew && hasConfirmed:
		return ErrOnlyNew
	case alreadyActivated:
		return ErrAlreadyUsed
	case (place == PlaceMenu) != (p.Kind == KindDays):
		return ErrWrongPlace
	}
	return nil
}

// Apply — цена со скидкой, не ниже 0; код на дни цену не меняет.
func Apply(base int, p model.Promo) int {
	price := base
	switch p.Kind {
	case KindPercent:
		price = base * (100 - p.Value) / 100
	case KindRub:
		price = base - p.Value
	case KindDays:
	}
	return max(price, 0)
}
