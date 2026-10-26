package config

type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379" validate:"required"`
	Password string `env:"REDIS_PASSWORD"`
	Database int    `env:"REDIS_DB" envDefault:"0" validate:"gte=0"`
}
