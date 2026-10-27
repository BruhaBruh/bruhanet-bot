package config

type ObservabilityConfig struct {
	Addr string `env:"OBSERVABILITY_ADDR" envDefault:":8081" validate:"required"`
}
