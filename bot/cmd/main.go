package main

import (
	"context"
	"log/slog"
	"os"
	_ "time/tzdata"

	"github.com/bruhabruh/bruhanet/bot/internal/app"
	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/store"
	executor "github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/cfg"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func main() {
	// Docker HEALTHCHECK: до загрузки конфига, только env.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		addr := os.Getenv("OBSERVABILITY_ADDR")
		if addr == "" {
			addr = ":8081"
		}
		os.Exit(observability.Healthcheck(addr))
	}
	// Одноразовый контейнер миграций: нужен только DATABASE_URL, без остального конфига бота.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		stlog.New()
		os.Exit(migrate())
	}
	// JSON с level=ERROR ещё до конфига: ошибки загрузки должны дойти до алерта.
	stlog.New()
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

func migrate() int {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		slog.Error("DATABASE_URL is required")
		return 1
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		slog.Error("failed to open postgres", "error", err)
		return 1
	}
	defer st.Close()
	if err = st.Migrate(ctx); err != nil {
		slog.Error("failed to migrate", "error", err)
		return 1
	}
	slog.Info("migrations applied")
	return 0
}
