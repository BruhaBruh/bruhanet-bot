package store

import "errors"

var (
	ErrOpen          = errors.New("open postgres")
	ErrMigrate       = errors.New("migrate postgres")
	ErrNotFound      = errors.New("not found")
	ErrNotPending    = errors.New("payment is not pending")
	ErrInvalidStatus = errors.New("invalid payment status")
	ErrEmptyMessage  = errors.New("outbox message needs event type and chat")
)
