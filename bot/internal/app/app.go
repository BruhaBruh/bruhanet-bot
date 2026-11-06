package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/handler"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/outbox"
	"github.com/bruhabruh/bruhanet/bot/internal/reminder"
	"github.com/bruhabruh/bruhanet/core/panelsync"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	contentTTL    = 10 * time.Second
	panelSyncCron = "*/15 * * * *"
)

func New(logger *slog.Logger, config *config.Config) (app.App, error) {
	fatal := make(chan error, 1)

	logger = logger.With("service", "bot")

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Redis.Addr,
		DB:       config.Redis.Database,
		Password: config.Redis.Password,
	})

	st, err := store.Open(
		observability.WithProbe(context.Background()), config.Database.URL,
		store.WithTracer(observability.PostgresTracer()),
	)
	if err != nil {
		return nil, err
	}
	if err = st.CheckSchema(context.Background()); err != nil {
		st.Close()
		return nil, err
	}

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

	if err = observability.RegisterBackupGauge(st.LastBackupAt); err != nil {
		st.Close()
		return nil, errors.Join(errors.New("failed to register backup gauge"), err)
	}

	rdb.AddHook(observability.RedisHook())
	if err = warnLegacyPayments(stlog.Wrap(context.Background(), logger), rdb); err != nil {
		logger.Warn("failed to scan legacy fsm payments", "error", err)
	}
	observability.InitBusinessSeries(slices.Collect(maps.Keys(config.Payment.Prices)))

	// Обёртка только для хендлеров и reminder; readiness ходит напрямую.
	rwObs := observability.InstrumentRemnawave(rw)

	var inflight sync.WaitGroup

	// env PAYMENT_*/SUPPORT_URL — значения по умолчанию, правки из админки доходят за contentTTL
	defaults, cronWarning := settings.Defaults(config.Payment.Prices, config.Payment.Phone, config.Payment.Bank, config.Support.URL).WithCron(config.Notification.Cron)
	if cronWarning != "" {
		logger.Warn(cronWarning)
	}
	cnt := content.New(st, defaults, contentTTL, nil)
	b, err := handler.New(logger, config, handler.Deps{FSM: fsm.NewRedis(rdb), InFlight: &inflight, Remnawave: rwObs, Payments: st, Content: cnt})
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

	ow := outbox.New(st, b)
	outboxDone := make(chan struct{})
	if err = observability.Registry.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "bot_outbox_pending", Help: "Messages waiting in the outbox.",
	}, func() float64 {
		ctx, cancel := context.WithTimeout(observability.WithProbe(context.Background()), 2*time.Second)
		defer cancel()
		n, err := st.OutboxCount(ctx)
		if err != nil {
			return math.NaN()
		}
		return float64(n)
	})); err != nil {
		return nil, errors.Join(errors.New("failed to register outbox gauge"), err)
	}

	rem := reminder.New(rwObs, st, func(ctx context.Context, k reminder.Kind, exp time.Time, sub string) (string, *models.InlineKeyboardMarkup) {
		return routes.ReminderMessage(ctx, cnt, string(k), exp, sub)
	}, rdb, func(ctx context.Context) (time.Duration, time.Duration) {
		s := cnt.Settings(ctx)
		return time.Duration(s.ReminderWeekDays) * 24 * time.Hour, time.Duration(s.ReminderDayHours) * time.Hour
	}, nil)
	cl := cronLogger(logger)
	c := cron.New(
		cron.WithLocation(loc),
		cron.WithChain(cron.Recover(cl), cron.SkipIfStillRunning(cl)),
	)

	runCtx, cancelRun := context.WithCancel(context.Background())
	done := make(chan struct{})

	remJob := &reminderJob{c: c, run: func() { rem.Run(stlog.Wrap(runCtx, logger, "component", "reminder")) }}
	if err = remJob.reschedule(cnt.Settings(context.Background()).ReminderTime); err != nil {
		cancelRun()
		return nil, errors.Join(errors.New("failed to schedule reminders"), err)
	}

	if _, err = c.AddFunc(panelSyncCron, func() { syncPanel(stlog.Wrap(runCtx, logger, "component", "panel_sync"), rwObs, st) }); err != nil {
		cancelRun()
		return nil, errors.Join(errors.New("failed to schedule panel sync"), err)
	}

	// Проверки зависимостей: WithProbe выключает спаны и bot_external_* для проб (иначе 30с-проба плодит мусорные трейсы).
	ready := observability.NewReadiness(map[string]observability.Check{
		"redis":    func(ctx context.Context) error { return rdb.Ping(observability.WithProbe(ctx)).Err() },
		"postgres": func(ctx context.Context) error { return st.Ping(observability.WithProbe(ctx)) },
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
		// перед bot: останавливается после него, когда хендлеры уже дренированы
		app.WithComponent("postgres", lifecycle.New(
			func(context.Context) error { return nil },
			func(context.Context) error {
				st.Close()
				return nil
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
		app.WithComponent("outbox", lifecycle.New(
			func(context.Context) error {
				go func() {
					defer close(outboxDone)
					ow.Run(runCtx)
				}()
				return nil
			},
			func(ctx context.Context) error {
				cancelRun()
				select {
				case <-outboxDone:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)),
		app.WithComponent("reminder", lifecycle.New(
			func(context.Context) error {
				c.Start()
				go remJob.follow(stlog.Wrap(runCtx, logger, "component", "reminder"), contentTTL, func(ctx context.Context) string {
					return cnt.Settings(ctx).ReminderTime
				})
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

// reminderJob держит задачу напоминаний на времени из настроек.
type reminderJob struct {
	c   *cron.Cron
	run func()
	id  cron.EntryID
	at  string
}

// reschedule ставит задачу на at (HH:MM по зоне cron); при ошибке прежняя задача остаётся.
func (j *reminderJob) reschedule(at string) error {
	if at == j.at {
		return nil
	}
	t, err := time.Parse("15:04", at)
	if err != nil {
		return err
	}
	id, err := j.c.AddFunc(fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour()), j.run)
	if err != nil {
		return err
	}
	if j.id != 0 {
		j.c.Remove(j.id)
	}
	j.id, j.at = id, at
	return nil
}

// follow переставляет задачу, когда время в настройках изменилось.
func (j *reminderJob) follow(ctx context.Context, every time.Duration, at func(context.Context) string) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			next := at(ctx)
			if next == j.at {
				continue
			}
			if err := j.reschedule(next); err != nil {
				stlog.Error(ctx, "failed to reschedule reminders", "error", err, "time", next)
				continue
			}
			stlog.Info(ctx, "reminders rescheduled", "time", next)
		}
	}
}

// syncPanel — один проход по cron. Занято ручным запуском из админки — не сбой, данные и так свежие.
func syncPanel(ctx context.Context, panel panelsync.Panel, st panelsync.Store) {
	res, err := panelsync.Run(ctx, panel, st, time.Now)
	switch {
	case errors.Is(err, store.ErrSyncRunning):
		stlog.Info(ctx, "panel sync skipped: already running")
	case err != nil:
		observability.PanelSyncFailures.Inc()
		stlog.Error(ctx, "panel sync failed", "error", err)
	default:
		stlog.Info(ctx, "panel sync done", "total", res.Total, "updated", res.Updated)
	}
}
