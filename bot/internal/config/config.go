package config

import coreconfig "github.com/bruhabruh/bruhanet/core/config"

type Config struct {
	Support       SupportConfig
	Payment       PaymentConfig
	Logger        coreconfig.LoggerConfig
	Telegram      coreconfig.TelegramConfig
	Remnawave     coreconfig.RemnawaveConfig
	Redis         RedisConfig
	Database      coreconfig.DatabaseConfig
	Notification  NotificationConfig
	Observability ObservabilityConfig
}
