package payment

import "errors"

var (
	ErrNotFound          = errors.New("payment not found")
	ErrFailedToMarshal   = errors.New("failed to marshal payment")
	ErrFailedToUnmarshal = errors.New("failed to unmarshal payment")
)
