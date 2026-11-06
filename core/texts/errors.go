package texts

import (
	"errors"
	"strings"

	"github.com/bruhabruh/bruhanet/core/usererr"
)

var (
	ErrInvalidMarkup = errors.New("invalid telegram html")
	ErrTooLong       = errors.New("text is too long")
	ErrBadButton     = errors.New("invalid button text")
)

// Problem — человеческое (русское) описание ошибки Validate без служебного префикса сентинела.
// Берётся usererr.Message; ошибка без него даёт usererr.Fallback, err.Error() человеку не уходит.
func Problem(err error) string {
	es := []error{err}
	if j, ok := err.(interface{ Unwrap() []error }); ok { //nolint:errorlint // errors.Join: [сентинел, подробность]
		if u := j.Unwrap(); len(u) > 1 {
			es = u[1:]
		}
	}
	msgs := make([]string, len(es))
	for i, e := range es {
		msgs[i] = usererr.Message(e)
	}
	return strings.Join(msgs, "\n")
}
