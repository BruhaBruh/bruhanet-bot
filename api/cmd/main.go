package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
	_ "time/tzdata"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bruhabruh/bruhanet/api/internal/config"
	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/api/internal/telegram"
	"github.com/bruhabruh/bruhanet/api/internal/web"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/app"
	"github.com/bruhabruh/bruhanet/pkg/cfg"
	"github.com/bruhabruh/bruhanet/pkg/lifecycle"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func main() {
	// Docker HEALTHCHECK: до загрузки конфига, только env.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		addr := os.Getenv("API_OBSERVABILITY_ADDR")
		if addr == "" {
			addr = ":8091"
		}
		os.Exit(healthcheck(addr))
	}
	stlog.New()
	conf, err := cfg.Load[config.Config]()
	if err == nil {
		err = cfg.Validate(conf)
	}
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	logger := stlog.New(
		stlog.WithLevel(conf.Logger.Level),
		stlog.WithSource(conf.Logger.WithSource),
		stlog.WithJSON(conf.Logger.JSON),
		stlog.WithDefault(true),
	).With("service", "api")

	a, err := newApp(logger, conf)
	if err != nil {
		logger.Error("failed to create application", "error", err)
		os.Exit(1)
	}
	if err := app.Run(a); err != nil {
		logger.Error("failed to run application", "error", err)
		os.Exit(1)
	}
}

func newApp(logger *slog.Logger, conf *config.Config) (app.App, error) {
	st, err := store.Open(context.Background(), conf.Database.URL)
	if err != nil {
		return nil, err
	}
	if err = st.CheckSchema(context.Background()); err != nil {
		st.Close()
		return nil, err
	}
	// ponytail: метрики внешних вызовов api — когда понадобятся
	panel, err := remnawave.New(conf.Remnawave.PanelURL, conf.Remnawave.Token, conf.Remnawave.Cookie)
	if err != nil {
		st.Close()
		return nil, errors.Join(errors.New("failed to create remnawave api"), err)
	}

	tg, err := telegram.New(conf.Telegram.Token)
	if err != nil {
		st.Close()
		return nil, errors.Join(errors.New("failed to create telegram client"), err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(server.Requests, collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	obsMux := http.NewServeMux()
	obsMux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	obsMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	obs := &http.Server{Addr: conf.ObservabilityAddr, Handler: obsMux, ReadHeaderTimeout: 5 * time.Second}

	ctx := stlog.Wrap(context.Background(), logger)
	defaults, cronWarning := settings.Defaults(conf.Payment.Prices, conf.Payment.Phone, conf.Payment.Bank, conf.Support.URL).WithCron(conf.Notification.Cron)
	if cronWarning != "" {
		logger.Warn(cronWarning)
	}
	srv := &http.Server{
		Addr: conf.Addr,
		Handler: server.New(server.Deps{
			Store:    st,
			Panel:    panel,
			Telegram: tg,
			Defaults: defaults,
			Squad:    conf.Remnawave.InternalSquadId,
			AdminID:  conf.Telegram.AdminID,
			BaseURL:  conf.Admin.BaseURL,
			Now:      time.Now,
			Static:   web.FS(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	fatal := make(chan error, 2)
	serve := func(name string, s *http.Server) lifecycle.Lifecycle {
		return lifecycle.New(
			func(context.Context) error {
				go func() {
					if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
						fatal <- errors.Join(errors.New(name), err)
					}
				}()
				return nil
			},
			s.Shutdown,
		)
	}

	purgeCtx, stopPurge := context.WithCancel(ctx)
	purgeDone := make(chan struct{})

	return app.New(
		app.WithLogger(logger),
		app.WithFatal(fatal),
		// первым в списке = останавливается последним: сервер уже не ходит в базу
		app.WithComponent("postgres", lifecycle.New(
			func(context.Context) error { return nil },
			func(context.Context) error { st.Close(); return nil },
		)),
		app.WithComponent("observability", serve("observability server", obs)),
		app.WithComponent("purge", lifecycle.New(
			func(context.Context) error {
				go func() {
					defer close(purgeDone)
					purgeAuth(purgeCtx, st)
				}()
				return nil
			},
			func(ctx context.Context) error {
				stopPurge()
				select {
				case <-purgeDone:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)),
		app.WithComponent("http", serve("http server", srv)),
		app.WithStarted(func() { logger.Info("service started", "addr", conf.Addr) }),
	), nil
}

// purgeAuth чистит истёкшие токены входа и сессии: сразу и раз в час.
func purgeAuth(ctx context.Context, st *store.Store) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := st.PurgeAuth(ctx, time.Now()); err != nil && ctx.Err() == nil {
			stlog.Error(ctx, "failed to purge auth", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// healthcheck для docker HEALTHCHECK: 0 при 200 от /healthz, иначе 1.
func healthcheck(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// хост зафиксирован, из env только порт
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", nil) //nolint:gosec // G704 false positive
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G704 false positive: see above
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
