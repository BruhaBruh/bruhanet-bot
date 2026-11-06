package config

import coreconfig "github.com/bruhabruh/bruhanet/core/config"

type Config struct {
	Database          coreconfig.DatabaseConfig
	Telegram          coreconfig.TelegramConfig
	Remnawave         coreconfig.RemnawaveConfig
	Logger            coreconfig.LoggerConfig
	Admin             coreconfig.AdminConfig
	Payment           coreconfig.PaymentConfig
	Support           coreconfig.SupportConfig
	Notification      coreconfig.NotificationConfig
	Addr              string `env:"API_ADDR" envDefault:":8090" validate:"required"`
	ObservabilityAddr string `env:"API_OBSERVABILITY_ADDR" envDefault:":8091" validate:"required"`
}
