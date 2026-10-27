package app

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/reminder"
	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
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

	rdb.AddHook(observability.RedisHook())
	observability.InitBusinessSeries(slices.Collect(maps.Keys(config.Payment.Prices)))

	// Обёртка только для хендлеров и reminder; readiness ходит напрямую.
	rwObs := observability.InstrumentRemnawave(rw)

	var inflight sync.WaitGroup

	b, err := handler.New(logger, config, handler.Deps{FSM: fsm.NewRedis(rdb), InFlight: &inflight, Remnawave: rwObs})
	if err != nil {
		return nil, errors.Join(
			errors.New("failed to create telegram bot"),
			err,
		)
	}

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return nil, errors.Join(errors.New("failed to load Europe/Moscow"), err)
	}

	rem := reminder.New(rwObs, handler.NewNotifier(b), rdb, nil)
	cl := cronLogger(logger)
	c := cron.New(
		cron.WithLocation(loc),
		cron.WithChain(cron.Recover(cl), cron.SkipIfStillRunning(cl)),
	)

	runCtx, cancelRun := context.WithCancel(context.Background())
	done := make(chan struct{})

	if _, err = c.AddFunc(config.Notification.Cron, func() { rem.Run(stlog.Wrap(runCtx, logger, "component", "reminder")) }); err != nil {
		cancelRun()
		return nil, errors.Join(errors.New("failed to schedule reminders"), err)
	}

	// Проверки зависимостей: WithProbe выключает спаны и bot_external_* для проб (иначе 30с-проба плодит мусорные трейсы).
	ready := observability.NewReadiness(map[string]observability.Check{
		"redis":    func(ctx context.Context) error { return rdb.Ping(observability.WithProbe(ctx)).Err() },
		"telegram": func(ctx context.Context) error { _, err := b.GetMe(observability.WithProbe(ctx)); return err },
		"remnawave": func(ctx context.Context) error {
			// 401/403/5xx/сеть — ошибка; «юзер не найден» — панель жива.
			if _, err := rw.GetUser(ctx, config.Telegram.AdminID); err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
				return err
			}
			return nil
		},
	}, 30*time.Second, 5*time.Second)
	obs := observability.NewServer(config.Observability.Addr, ready)

	shutdownTracing, err := observability.InitTracing(context.Background())
	if err != nil {
		cancelRun()
		return nil, errors.Join(errors.New("failed to init tracing"), err)
	}

	return app.New(
		app.WithLogger(logger),
		app.WithFatal(fatal),
		// первым в списке = останавливается последним: хвост спанов успевает уйти
		app.WithComponent("tracing", lifecycle.New(
			func(context.Context) error { return nil },
			shutdownTracing,
		)),
		app.WithComponent("observability", lifecycle.New(
			func(context.Context) error {
				go func() {
					if err := obs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
						select {
						case fatal <- errors.Join(errors.New("observability server"), err):
						default:
						}
					}
				}()
				go ready.Run(runCtx)
				return nil
			},
			func(ctx context.Context) error {
				return obs.Shutdown(ctx)
			},
		)),
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
		app.WithComponent("reminder", lifecycle.New(
			func(context.Context) error {
				c.Start()
				return nil
			},
			func(ctx context.Context) error {
				cancelRun()
				select {
				case <-c.Stop().Done():
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
