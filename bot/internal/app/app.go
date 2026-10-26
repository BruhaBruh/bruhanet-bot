package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler"
	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
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

	rw, err := remnawave.New(
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

	var inflight sync.WaitGroup

	b, err := handler.New(logger, config, handler.Deps{FSM: fsm.NewRedis(rdb), InFlight: &inflight, Remnawave: rw})
	if err != nil {
		return nil, errors.Join(
			errors.New("failed to create telegram bot"),
			err,
		)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	done := make(chan struct{})

	return app.New(
		app.WithLogger(logger),
		app.WithFatal(fatal),
		app.WithComponent("redis", lifecycle.New(
			func(ctx context.Context) error {
				return rdb.Ping(runCtx).Err()
			},
			func(context.Context) error {
				return rdb.Close()
			},
		)),
		app.WithComponent("bot", lifecycle.New(
			func(context.Context) error {
				go func() {
					defer close(done)
					b.Start(runCtx)
				}()
				return nil
			},
			func(ctx context.Context) error {
				cancelRun()
				select {
				case <-done:
				case <-ctx.Done():
					return ctx.Err()
				}
				drained := make(chan struct{})
				go func() {
					inflight.Wait()
					close(drained)
				}()
				select {
				case <-drained:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)),
		app.WithStarted(func() {
			logger.Info("service started")
		}),
	), nil
}
