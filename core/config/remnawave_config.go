package config

import "github.com/google/uuid"

type RemnawaveConfig struct {
	PanelURL        string    `env:"REMNAWAVE_PANEL_URL" validate:"required,http_url"`
	Token           string    `env:"REMNAWAVE_API_TOKEN" validate:"required"`
	InternalSquadId uuid.UUID `env:"REMNAWAVE_INTERNAL_SQUAD_ID" validate:"required,uuid4"`
	Cookie          string    `env:"REMNAWAVE_COOKIE"`
}
