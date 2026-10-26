package routes_test

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler"
	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

const (
	adminID int64 = 1
	userID  int64 = 42
)

var now = time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)

type env struct {
	b   *bot.Bot
	srv *tgtest.Server
	rw  *tgtest.Remnawave
	fsm fsm.Storage
	log *bytes.Buffer
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWithStore(t, fsm.NewInMemory())
}

func newEnvWithStore(t *testing.T, store fsm.Storage) *env {
	t.Helper()
	logger, buf := tgtest.Logger()
	e := &env{
		srv: tgtest.NewServer(t),
		rw:  tgtest.NewRemnawave(func() time.Time { return now }),
		fsm: store,
		log: buf,
	}
	cfg := &config.Config{
		Telegram: config.TelegramConfig{Token: tgtest.Token, AdminID: adminID},
		Support:  config.SupportConfig{URL: "https://t.me/support"},
		Payment: config.PaymentConfig{
			Phone:  "+7 (800) 555-35-35",
			Bank:   "Т-Банк",
			Prices: map[int]int{1: 150, 6: 800, 12: 1600},
		},
		Remnawave: config.RemnawaveConfig{InternalSquadId: uuid.New()},
	}
	b, err := handler.New(logger, cfg, handler.Deps{
		FSM:       store,
		InFlight:  &sync.WaitGroup{},
		Remnawave: e.rw,
		Now:       func() time.Time { return now },
	}, e.srv.Options()...)
	if err != nil {
		t.Fatalf("handler.New: %v", err)
	}
	e.b = b
	return e
}

func (e *env) process(t *testing.T, u *models.Update) {
	t.Helper()
	e.b.ProcessUpdate(t.Context(), u)
}

func (e *env) state(t *testing.T, id int64) fsm.State {
	t.Helper()
	st, err := fsm.For(e.fsm, id).State(t.Context())
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	return st
}

func (e *env) payment(t *testing.T, id int64) model.Payment {
	t.Helper()
	p, err := fsm.Data[model.Payment](t.Context(), fsm.For(e.fsm, id))
	if err != nil {
		t.Fatalf("Data: %v", err)
	}
	return p
}

func (e *env) count(method string) int {
	return len(e.srv.Calls(method))
}

func (e *env) last(t *testing.T, method string) tgtest.Call {
	t.Helper()
	calls := e.srv.Calls(method)
	if len(calls) == 0 {
		t.Fatalf("нет вызовов %s", method)
	}
	return calls[len(calls)-1]
}
