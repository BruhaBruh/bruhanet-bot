package main

import (
	"log/slog"
	"os"

	"github.com/bruhabruh/bruhanet/bot/internal/app"
	"github.com/bruhabruh/bruhanet/bot/internal/config"
	executor "github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/cfg"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func main() {
	config, err := cfg.Load[config.Config]()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if err = cfg.Validate(config); err != nil {
		slog.Error("failed to validate config", "error", err)
		os.Exit(1)
	}
	logger := stlog.New(
		stlog.WithLevel(config.Logger.Level),
		stlog.WithSource(config.Logger.WithSource),
		stlog.WithJSON(config.Logger.JSON),
		stlog.WithDefault(true),
	)

	app, err := app.New(logger, config)
	if err != nil {
		logger.Error(
			"failed to create application",
			"error", err,
		)
		os.Exit(1)
	}
	if err := executor.Run(app); err != nil {
		logger.Error(
			"failed to run application",
			"error", err,
		)
		os.Exit(1)
	}
}
