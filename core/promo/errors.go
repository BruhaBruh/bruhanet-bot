package promo

import "errors"

// Причины отказа в порядке проверки (Check возвращает первую).
var (
	ErrNotFound    = errors.New("promo not found")
	ErrDisabled    = errors.New("promo disabled")
	ErrNotStarted  = errors.New("promo not started")
	ErrExpired     = errors.New("promo expired")
	ErrExhausted   = errors.New("promo activations exhausted")
	ErrNotYours    = errors.New("promo is personal for another user")
	ErrOnlyNew     = errors.New("promo is for new users only")
	ErrAlreadyUsed = errors.New("promo already used by this user")
	ErrWrongPlace  = errors.New("promo kind does not fit this place")

	// ErrInvalid — новый код не прошёл ValidateNew; текст причины — в обёрнутой ошибке.
	ErrInvalid = errors.New("invalid promo")
)
