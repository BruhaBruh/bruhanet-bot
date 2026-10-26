package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler"
	"github.com/bruhabruh/bruhanet/bot/internal/storage/payment"
	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func New(logger *slog.Logger, config *config.Config) (app.App, error) {
	fatal := make(chan error, 1)

	logger = logger.With("service", "bot")

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		DB:       config.Redis.Database,
		Password: config.Redis.Password,
	})

	_, err := remnawave.New(
		config.Remnawave.PanelURL,
		config.Remnawave.Token,
		config.Remnawave.Cookie,
	)
	if err != nil {
		return nil, errors.Join(
			errors.New("failed to create remnawave api"),
			err,
		)
	}
	_ = payment.New(rdb)

	bot, err := handler.New(logger, config)
	if err != nil {
		return nil, errors.Join(
			errors.New("failed to create telegram bot"),
			err,
		)
	}

	return app.New(
		app.WithLogger(logger),
		app.WithFatal(fatal),
		app.WithComponent("redis", lifecycle.New(
			func(ctx context.Context) error {
				return rdb.Ping(ctx).Err()
			},
			func(ctx context.Context) error {
				rdb.Close()

				return nil
			},
		)),
		app.WithComponent("bot", lifecycle.New(
			func(ctx context.Context) error {
				go func() {
					bot.Start(context.Background())
				}()
				return nil
			},
			func(ctx context.Context) error {
				_, err := bot.Close(ctx)
				return err
			},
		)),
		app.WithStarted(func() {
			logger.Info("service started")
		}),
	), nil
}
