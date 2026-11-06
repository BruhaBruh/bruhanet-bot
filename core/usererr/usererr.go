// Package usererr — ошибка с двумя текстами: английский Error() для логов и errors.Is,
// русский Message() для человека (панель). Показывать err.Error() человеку нельзя.
package usererr

import (
	"errors"
	"fmt"
)

// Fallback — текст для ошибки без Message().
const Fallback = "Некорректное значение"

// Error — английская причина для разработчика и русское сообщение для админа.
type Error struct{ En, Ru string }

func (e *Error) Error() string   { return e.En }
func (e *Error) Message() string { return e.Ru }

// Errorf форматирует оба текста одними и теми же аргументами.
func Errorf(en, ru string, args ...any) error {
	return &Error{En: fmt.Sprintf(en, args...), Ru: fmt.Sprintf(ru, args...)}
}

// Message — русское сообщение из цепочки err; нет его — Fallback.
func Message(err error) string {
	if m, ok := errors.AsType[interface {
		error
		Message() string
	}](err); ok {
		return m.Message()
	}
	return Fallback
}
