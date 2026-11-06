package config

import coreconfig "github.com/bruhabruh/bruhanet/core/config"

type Config struct {
	Support       coreconfig.SupportConfig
	Payment       coreconfig.PaymentConfig
	Logger        coreconfig.LoggerConfig
	Telegram      coreconfig.TelegramConfig
	Admin         coreconfig.AdminConfig
	Remnawave     coreconfig.RemnawaveConfig
	Redis         RedisConfig
	Database      coreconfig.DatabaseConfig
	Notification  coreconfig.NotificationConfig
	Observability ObservabilityConfig
}
