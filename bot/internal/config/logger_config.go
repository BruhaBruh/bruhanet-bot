package config

type LoggerConfig struct {
	Level      string `env:"LOG_LEVEL" envDefault:"info" validate:"oneof=debug info warn error"`
	WithSource bool   `env:"LOG_SOURCE" envDefault:"true"`
	JSON       bool   `env:"LOG_JSON" envDefault:"true"`
}
