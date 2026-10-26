package cfg

import "errors"

var (
	ErrFailedParseEnvVariables = errors.New("failed to parse environment variables")
	ErrInvalidField            = errors.New("invalid field")
	ErrInvalidConfig           = errors.New("invalid config")
)
