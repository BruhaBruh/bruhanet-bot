package store

import "errors"

var (
	ErrOpen           = errors.New("open postgres")
	ErrMigrate        = errors.New("migrate postgres")
	ErrSchemaOutdated = errors.New("schema is outdated: run `bot migrate`")
	ErrNotFound       = errors.New("not found")
	ErrNotPending     = errors.New("payment is not pending")
	ErrInvalidStatus  = errors.New("invalid payment status")
	ErrEmptyMessage   = errors.New("outbox message needs event type and chat")
	ErrTokenInvalid   = errors.New("login token is invalid, used or expired")
	ErrPromoExists    = errors.New("promo code already exists")
	ErrSyncRunning    = errors.New("panel sync is already running")
)
