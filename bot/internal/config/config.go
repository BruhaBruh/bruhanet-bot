package config

type Config struct {
	Support      SupportConfig
	Payment      PaymentConfig
	Logger       LoggerConfig
	Telegram     TelegramConfig
	Remnawave    RemnawaveConfig
	Redis        RedisConfig
	Notification NotificationConfig
}
