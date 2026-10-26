package config

type TelegramConfig struct {
	Token   string `env:"TELEGRAM_TOKEN" validate:"required"`
	AdminID int64  `env:"TELEGRAM_ADMIN_ID" validate:"required"`
}
