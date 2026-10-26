package cfg

import (
	"errors"

	env "github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

func Load[T any]() (*T, error) {
	var cfg T
	if err := env.Parse(&cfg); err != nil {
		return nil, errors.Join(ErrFailedParseEnvVariables, err)
	}
	return &cfg, nil
}

func Validate(cfg any) error {
	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(cfg); err != nil {
		if validationErrs, ok := errors.AsType[validator.ValidationErrors](err); ok {
			return errors.Join(ErrInvalidField, validationErrs)
		}
		return errors.Join(ErrInvalidConfig, err)
	}
	return nil
}
