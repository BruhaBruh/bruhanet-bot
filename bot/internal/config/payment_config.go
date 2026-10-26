package config

type PaymentConfig struct {
	Phone  string      `env:"PAYMENT_PHONE" validate:"required"`
	Bank   string      `env:"PAYMENT_BANK" validate:"required"`
	Prices map[int]int `env:"PAYMENT_PRICES" validate:"required"`
}
