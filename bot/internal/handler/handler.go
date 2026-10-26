package handler

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/router"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

type Deps struct {
	FSM fsm.Storage
	// InFlight считает выполняющиеся хендлеры; app дожидается его при остановке.
	InFlight *sync.WaitGroup
}

func New(logger *slog.Logger, cfg *config.Config, deps Deps, opts ...bot.Option) (*bot.Bot, error) {
	if deps.FSM == nil || deps.InFlight == nil {
		return nil, errors.New("handler: deps.FSM and deps.InFlight are required")
	}
	r := router.New()
	routes.Register(r, routes.Deps{Config: cfg})

	options := append([]bot.Option{
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
