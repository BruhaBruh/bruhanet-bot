package callback

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// maxNameLen гарантирует лимит Telegram: 40 + ":" + 20 (min int64) = 61 ≤ 64 байт.
const (
	maxNameLen = 40
	sep        = ":"
)

var (
	ErrInvalidName = errors.New("invalid callback name")
	ErrInvalidData = errors.New("invalid callback data")
)

var nameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

type Arg interface {
	~int | ~int64
}

type Route[T Arg] struct {
	name string
}

func ValidateName(name string) error {
	if len(name) > maxNameLen || !nameRe.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

func New[T Arg](name string) Route[T] {
	if err := ValidateName(name); err != nil {
		panic(err)
	}
	return Route[T]{name: name}
}

func (r Route[T]) Name() string {
	return r.name
}

func (r Route[T]) Data(v T) string {
	return r.name + sep + strconv.FormatInt(int64(v), 10)
}

func (r Route[T]) Parse(data string) (T, error) {
	raw, ok := strings.CutPrefix(data, r.name+sep)
	if !ok {
		return 0, fmt.Errorf("%w: %q is not %s", ErrInvalidData, data, r.name)
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %w", ErrInvalidData, data, err)
	}
	return T(n), nil
}

// maxName2Len: 22 + ":" + 20 + ":" + 20 = 64 байт.
const maxName2Len = 22

// Route2 — роут с двумя целыми аргументами: name:a:b.
type Route2[A, B Arg] struct {
	name string
}

func New2[A, B Arg](name string) Route2[A, B] {
	if err := ValidateName(name); err != nil {
		panic(err)
	}
	if len(name) > maxName2Len {
		panic(fmt.Errorf("%w: %q длиннее %d", ErrInvalidName, name, maxName2Len))
	}
	return Route2[A, B]{name: name}
}

func (r Route2[A, B]) Name() string {
	return r.name
}

func (r Route2[A, B]) Data(a A, b B) string {
	return r.name + sep + strconv.FormatInt(int64(a), 10) + sep + strconv.FormatInt(int64(b), 10)
}

func (r Route2[A, B]) Parse(data string) (A, B, error) {
	raw, ok := strings.CutPrefix(data, r.name+sep)
	if !ok {
		return 0, 0, fmt.Errorf("%w: %q is not %s", ErrInvalidData, data, r.name)
	}
	rawA, rawB, ok := strings.Cut(raw, sep)
	if !ok {
		return 0, 0, fmt.Errorf("%w: %q: want two args", ErrInvalidData, data)
	}
	a, errA := strconv.ParseInt(rawA, 10, 64)
	b, errB := strconv.ParseInt(rawB, 10, 64)
	if err := errors.Join(errA, errB); err != nil {
		return 0, 0, fmt.Errorf("%w: %q: %w", ErrInvalidData, data, err)
	}
	return A(a), B(b), nil
}
