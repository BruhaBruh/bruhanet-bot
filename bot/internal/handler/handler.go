package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/content"
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
	Payments  routes.Payments
	Content   *content.Provider
	// Now — источник времени; nil = time.Now (подменяется в тестах).
	Now func() time.Time
	// Fails — защита от перебора кодов подарка; nil — без лимита.
	Fails routes.FailCounter
}

func New(logger *slog.Logger, cfg *config.Config, deps Deps, opts ...bot.Option) (*bot.Bot, error) {
	if deps.FSM == nil || deps.InFlight == nil || deps.Remnawave == nil || deps.Payments == nil || deps.Content == nil {
		return nil, errors.New("handler: deps.FSM, deps.InFlight, deps.Remnawave, deps.Payments and deps.Content are required")
	}
	deps.Content.SetMiniApp(cfg.MiniApp.Origin)
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	r := router.New()
	routes.Register(r, routes.Deps{Config: cfg, FSM: deps.FSM, Remnawave: deps.Remnawave, Payments: deps.Payments, Content: deps.Content, Now: deps.Now, Fails: deps.Fails})

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
			middleware.TextUser,
			// снаружи Observe: запись в БД не попадает в длительность апдейта
			middleware.LastSeen(deps.Payments.TouchUser, middleware.LastSeenEvery, now),
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
