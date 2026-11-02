package store

import "errors"

var (
	ErrOpen    = errors.New("open postgres")
	ErrMigrate = errors.New("migrate postgres")
)
