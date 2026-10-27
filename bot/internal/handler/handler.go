package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/router"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

type Deps struct {
	FSM fsm.Storage
	// InFlight считает выполняющиеся хендлеры; app дожидается его при остановке.
	InFlight  *sync.WaitGroup
	Remnawave routes.Remnawave
	// Now — источник времени; nil = time.Now (подменяется в тестах).
	Now func() time.Time
}

func New(logger *slog.Logger, cfg *config.Config, deps Deps, opts ...bot.Option) (*bot.Bot, error) {
	if deps.FSM == nil || deps.InFlight == nil || deps.Remnawave == nil {
		return nil, errors.New("handler: deps.FSM, deps.InFlight and deps.Remnawave are required")
	}
	r := router.New()
	routes.Register(r, routes.Deps{Config: cfg, FSM: deps.FSM, Remnawave: deps.Remnawave, Now: deps.Now})

	options := append([]bot.Option{
		// pollTimeout и Timeout клиента — как у go-telegram по умолчанию (1 минута)
		bot.WithHTTPClient(time.Minute, &http.Client{
			Timeout:   time.Minute,
			Transport: observability.TelegramTransport(http.DefaultTransport),
		}),
		bot.WithDefaultHandler(r.HandleDefault),
		bot.WithErrorsHandler(func(err error) {
			logger.Error("telegram error", "error", err)
		}),
		bot.WithMiddlewares(
			middleware.Track(deps.InFlight),
			middleware.WithLogger(logger),
			middleware.Recovery,
			middleware.RequestID,
			middleware.TelegramID,
			middleware.Observe,
			middleware.Logger,
			middleware.AutoAnswer(),
			middleware.Recovery,
			fsm.Middleware(deps.FSM),
		),
	}, opts...)

	b, err := bot.New(cfg.Telegram.Token, options...)
	if err != nil {
		return nil, err
	}
	r.Install(b)
	return b, nil
}
