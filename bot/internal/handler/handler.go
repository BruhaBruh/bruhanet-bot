package handler

import (
	"log/slog"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
)

func New(logger *slog.Logger, config *config.Config) (*bot.Bot, error) {
	bot, err := bot.New(config.Telegram.Token)
	if err != nil {
		return nil, err
	}
	return bot, nil
}
