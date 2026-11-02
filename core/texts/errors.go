package texts

import "errors"

var (
	ErrInvalidMarkup = errors.New("invalid telegram html")
	ErrTooLong       = errors.New("text is too long")
	ErrBadButton     = errors.New("invalid button text")
)

// Problem — человеческое описание ошибки Validate без служебного префикса сентинела.
func Problem(err error) string {
	if j, ok := err.(interface{ Unwrap() []error }); ok { //nolint:errorlint // errors.Join: [сентинел, подробность]
		if es := j.Unwrap(); len(es) > 1 {
			return errors.Join(es[1:]...).Error()
		}
	}
	return err.Error()
}
