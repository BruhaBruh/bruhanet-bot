package config

type NotificationConfig struct {
	Cron string `env:"NOTIFICATION_CRON" validate:"required,cron"`
}
