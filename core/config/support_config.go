package config

type SupportConfig struct {
	URL string `env:"SUPPORT_URL" validate:"required,http_url"`
}
