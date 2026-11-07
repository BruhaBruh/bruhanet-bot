package remnawave

import "errors"

var (
	ErrFailedToMakeRequest = errors.New("failed to make request")
	ErrInvalidResponseType = errors.New("invalid response type")
	ErrUserNotFound        = errors.New("user not found")
	ErrDeviceNotFound      = errors.New("device not found")
	ErrSubpageConfigNotSet = errors.New("subscription page config not set")
)
