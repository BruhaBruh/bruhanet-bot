package config

import coreconfig "github.com/bruhabruh/bruhanet/core/config"

// MiniAppConfig — Mini App «Кабинет»: свой порт и origin (https), с которого он открывается. Пустой Addr — выключено.
type MiniAppConfig struct {
	Addr   string `env:"MINIAPP_ADDR"`
	Origin string `env:"MINIAPP_ORIGIN" validate:"required_with=Addr,omitempty,https_url"`
}

type Config struct {
	Database          coreconfig.DatabaseConfig
	Telegram          coreconfig.TelegramConfig
	Remnawave         coreconfig.RemnawaveConfig
	Logger            coreconfig.LoggerConfig
	Admin             coreconfig.AdminConfig
	Payment           coreconfig.PaymentConfig
	Support           coreconfig.SupportConfig
	Notification      coreconfig.NotificationConfig
	MiniApp           MiniAppConfig
	Addr              string `env:"API_ADDR" envDefault:":8090" validate:"required"`
	ObservabilityAddr string `env:"API_OBSERVABILITY_ADDR" envDefault:":8091" validate:"required"`
}
