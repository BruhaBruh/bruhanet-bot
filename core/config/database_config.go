package config

type DatabaseConfig struct {
	URL string `env:"DATABASE_URL" validate:"required"`
}
