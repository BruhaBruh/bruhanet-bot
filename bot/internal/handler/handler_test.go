package handler

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/reminder"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func newTestHandler(t *testing.T) (*bot.Bot, *tgtest.Server, *bytes.Buffer) {
	t.Helper()
	srv := tgtest.NewServer(t)
	logger, buf := tgtest.Logger()
	cfg := &config.Config{
		Telegram: config.TelegramConfig{Token: tgtest.Token, AdminID: 1},
		Support:  config.SupportConfig{URL: "https://t.me/support"},
	}
	rw := tgtest.NewRemnawave(time.Now)
	rw.AddUser(42, time.Now().Add(24*time.Hour))
	b, err := New(logger, cfg, Deps{FSM: fsm.NewInMemory(), InFlight: &sync.WaitGroup{}, Remnawave: rw}, srv.Options()...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b, srv, buf
}

func TestStartSendsMainMenu(t *testing.T) {
	b, srv, buf := newTestHandler(t)
	b.ProcessUpdate(t.Context(), tgtest.Message(42, "/start"))

	sends := srv.Calls("sendMessage")
	if len(sends) != 1 {
		t.Fatalf("sendMessage calls = %d", len(sends))
	}
	p := sends[0].Params
	if p.Get("chat_id") != "42" || !strings.Contains(p.Get("text"), "Test") || p.Get("parse_mode") != "MarkdownV2" {
		t.Fatalf("params = %v", p)
	}
	for _, want := range []string{
		`"callback_data":"info"`, `"callback_data":"pay"`, `"url":"https://t.me/support"`,
		`"style":"primary"`, `"style":"danger"`,
	} {
		if !strings.Contains(p.Get("reply_markup"), want) {
			t.Fatalf("reply_markup без %s: %s", want, p.Get("reply_markup"))
		}
	}
	line := tgtest.FindLog(t, buf, "update handled")
	if line["route"] != "default" || line["telegram_id"] != float64(42) {
		t.Fatalf("line = %v", line)
	}
}

func TestMenuCallback(t *testing.T) {
	b, srv, buf := newTestHandler(t)
	b.ProcessUpdate(t.Context(), tgtest.Callback(42, "menu"))
	if n := len(srv.Calls("editMessageText")); n != 1 {
		t.Fatalf("editMessageText calls = %d", n)
	}
	answers := srv.Calls("answerCallbackQuery")
	if len(answers) != 1 || answers[0].Params.Get("text") != "" {
		t.Fatalf("answers = %+v", answers)
	}
	if line := tgtest.FindLog(t, buf, "update handled"); line["route"] != "callback:menu" {
		t.Fatalf("line = %v", line)
	}
}

func TestUnknownCallbackShowsMenu(t *testing.T) {
	b, srv, buf := newTestHandler(t)
	b.ProcessUpdate(t.Context(), tgtest.Callback(42, "removed_route:7"))
	if n := len(srv.Calls("editMessageText")); n != 1 {
		t.Fatalf("editMessageText calls = %d", n)
	}
	if n := len(srv.Calls("answerCallbackQuery")); n != 1 {
		t.Fatalf("answerCallbackQuery calls = %d", n)
	}
	if line := tgtest.FindLog(t, buf, "update handled"); line["route"] != "default" {
		t.Fatalf("line = %v", line)
	}
}

func TestEscapesUserName(t *testing.T) {
	b, srv, _ := newTestHandler(t)
	u := tgtest.Message(42, "/start")
	u.Message.From.FirstName = "a_b"
	b.ProcessUpdate(t.Context(), u)
	if text := srv.Calls("sendMessage")[0].Params.Get("text"); !strings.Contains(text, `a\_b`) {
		t.Fatalf("имя не экранировано: %q", text)
	}
}

func TestNameFallbackToID(t *testing.T) {
	b, srv, _ := newTestHandler(t)
	u := tgtest.Message(42, "/start")
	u.Message.From.FirstName = ""
	b.ProcessUpdate(t.Context(), u)
	if text := srv.Calls("sendMessage")[0].Params.Get("text"); !strings.Contains(text, `\#42`) {
		t.Fatalf("ожидался fallback #42: %q", text)
	}
}

func TestNewRequiresDeps(t *testing.T) {
	logger, _ := tgtest.Logger()
	cfg := &config.Config{Telegram: config.TelegramConfig{Token: tgtest.Token}}
	if _, err := New(logger, cfg, Deps{InFlight: &sync.WaitGroup{}, Remnawave: tgtest.NewRemnawave(time.Now)}); err == nil {
		t.Fatal("nil FSM должен давать ошибку")
	}
	if _, err := New(logger, cfg, Deps{FSM: fsm.NewInMemory(), Remnawave: tgtest.NewRemnawave(time.Now)}); err == nil {
		t.Fatal("nil InFlight должен давать ошибку")
	}
	if _, err := New(logger, cfg, Deps{FSM: fsm.NewInMemory(), InFlight: &sync.WaitGroup{}}); err == nil {
		t.Fatal("nil Remnawave должен давать ошибку")
	}
}

func TestStartIgnoresEditedMessage(t *testing.T) {
	b, srv, _ := newTestHandler(t)
	u := tgtest.Message(42, "/start")
	u.EditedMessage, u.Message = u.Message, nil
	b.ProcessUpdate(t.Context(), u)
	if n := len(srv.Calls("sendMessage")); n != 0 {
		t.Fatalf("sendMessage calls = %d", n)
	}
}

func TestStartIgnoresGroupChat(t *testing.T) {
	b, srv, _ := newTestHandler(t)
	u := tgtest.Message(42, "/start")
	u.Message.Chat.Type = models.ChatTypeGroup
	b.ProcessUpdate(t.Context(), u)
	if n := len(srv.Calls("sendMessage")); n != 0 {
		t.Fatalf("sendMessage calls = %d", n)
	}
}

func TestNotifierRemind(t *testing.T) {
	exp := time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)
	const sub = "https://sub.example/abc"

	for _, tc := range []struct {
		kind reminder.Kind
		want string
	}{
		{reminder.Day, "в течение суток"},
		{reminder.Week, "скоро закончится"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			b, srv, _ := newTestHandler(t)
			if err := NewNotifier(b).Remind(t.Context(), 42, tc.kind, exp, sub); err != nil {
				t.Fatalf("Remind: %v", err)
			}
			sends := srv.Calls("sendMessage")
			if len(sends) != 1 {
				t.Fatalf("sendMessage = %d", len(sends))
			}
			p := sends[0].Params
			if p.Get("chat_id") != "42" || p.Get("parse_mode") != "MarkdownV2" || !strings.Contains(p.Get("text"), tc.want) {
				t.Fatalf("params = %v", p)
			}
			if m := p.Get("reply_markup"); !strings.Contains(m, `"pay"`) || !strings.Contains(m, sub) {
				t.Fatalf("reply_markup: %s", m)
			}
		})
	}
}

func TestNotifierRemindErrors(t *testing.T) {
	for _, tc := range []struct {
		code          int
		desc          string
		undeliverable bool
	}{
		{403, "Forbidden: bot was blocked by the user", true},
		{400, "Bad Request: chat not found", true},
		{429, "Too Many Requests: retry after 1", false},
		{500, "oops", false},
	} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			b, srv, _ := newTestHandler(t)
			srv.FailChat(42, tc.code, tc.desc)
			err := NewNotifier(b).Remind(t.Context(), 42, reminder.Week, time.Now(), "https://sub.example")
			if err == nil {
				t.Fatal("want error")
			}
			if got := errors.Is(err, reminder.ErrUndeliverable); got != tc.undeliverable {
				t.Fatalf("ErrUndeliverable = %v, want %v (%v)", got, tc.undeliverable, err)
			}
		})
	}
}
