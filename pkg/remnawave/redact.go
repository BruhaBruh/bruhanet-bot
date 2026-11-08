package remnawave

import "strings"

// redactedError — ошибка с замаскированным секретом в тексте; Unwrap оставляет исходную для errors.Is/As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redact убирает secret из текста ошибки: *url.Error печатает полный адрес запроса, а в адресе бывает shortUuid подписки.
func redact(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	return &redactedError{msg: strings.ReplaceAll(err.Error(), secret, "[redacted]"), err: err}
}
