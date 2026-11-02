package texts

import "errors"

var (
	ErrInvalidMarkup = errors.New("invalid telegram html")
	ErrTooLong       = errors.New("text is too long")
	ErrBadButton     = errors.New("invalid button text")
)
