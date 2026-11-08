package config

// MiniAppConfig — адрес Mini App «Кабинет» (https). Пусто — Mini App выключен: кнопок на него в боте нет.
type MiniAppConfig struct {
	Origin string `env:"MINIAPP_ORIGIN" validate:"omitempty,https_url"`
}
