package config

type AdminConfig struct {
	BaseURL string `env:"ADMIN_BASE_URL" validate:"required,http_url"`
}
