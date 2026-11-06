package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestMainMenuForExistingUser(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(48*time.Hour))
	e.process(t, tgtest.Message(userID, "/start"))
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	for _, want := range []string{
		`"callback_data":"info"`, `"callback_data":"pay"`,
		`"copy_text":{"text":"https://sub.example/42"}`, `"url":"https://t.me/support"`,
	} {
		if !strings.Contains(kb, want) {
			t.Fatalf("keyboard without %s: %s", want, kb)
		}
	}
}

func TestMainMenuWithoutUser(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(userID, "/start"))
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	if !strings.Contains(kb, "Купить") || !strings.Contains(kb, `"callback_data":"pay"`) || strings.Contains(kb, `"callback_data":"info"`) {
		t.Fatalf("keyboard = %s", kb)
	}
}

func TestMainMenuRemnawaveDown(t *testing.T) {
	e := newEnv(t)
	e.rw.FailWith(errors.New("panel is down"))
	e.process(t, tgtest.Message(userID, "/start"))
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	if !strings.Contains(kb, "Сервис не работает") || !strings.Contains(kb, `"callback_data":"menu"`) {
		t.Fatalf("keyboard = %s", kb)
	}
	if l := tgtest.FindLog(t, e.log, "failed to get remnawave user"); l["level"] != "ERROR" {
		t.Fatalf("line = %v", l)
	}
}

func TestMenuCallbackEditsMessage(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	e.process(t, tgtest.Callback(userID, "menu"))
	edit := e.last(t, "editMessageText")
	if edit.Params.Get("message_id") != "20" || edit.Params.Get("chat_id") != "42" {
		t.Fatalf("edit = %v", edit.Params)
	}
	if n := e.count("sendMessage"); n != 0 {
		t.Fatalf("sendMessage calls = %d", n)
	}
}

// /a удалена (рассылки в веб-админке): админ получает обычное меню, ничего не уходит в outbox.
func TestAdminCommandAIsPlainMenu(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(adminID, "/a"))
	if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"pay"`) {
		t.Fatalf("keyboard = %s", kb)
	}
	if st := e.state(t, adminID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if n := len(e.pay.Outbox()); n != 0 {
		t.Fatalf("outbox = %d", n)
	}
}

// Стейты удалённой рассылки могли остаться в Redis: админ не должен застрять.
func TestAdminStuckInLegacyBroadcastState(t *testing.T) {
	for _, st := range []fsm.State{"await_broadcast", "confirm_broadcast"} {
		e := newEnv(t)
		if err := fsm.For(e.fsm, adminID).Set(t.Context(), st); err != nil {
			t.Fatal(err)
		}
		e.process(t, tgtest.Message(adminID, "Привет"))
		if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"pay"`) {
			t.Fatalf("%s: keyboard = %s", st, kb)
		}
		if n := len(e.pay.Outbox()); n != 0 {
			t.Fatalf("%s: outbox = %d", st, n)
		}
	}
}
