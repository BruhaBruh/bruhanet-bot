package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
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
			t.Fatalf("клавиатура без %s: %s", want, kb)
		}
	}
}

func TestMainMenuWithoutUser(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(userID, "/start"))
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	if !strings.Contains(kb, "Купить") || !strings.Contains(kb, `"callback_data":"pay"`) || strings.Contains(kb, `"callback_data":"info"`) {
		t.Fatalf("клавиатура = %s", kb)
	}
}

func TestMainMenuRemnawaveDown(t *testing.T) {
	e := newEnv(t)
	e.rw.FailWith(errors.New("panel is down"))
	e.process(t, tgtest.Message(userID, "/start"))
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	if !strings.Contains(kb, "Сервис не работает") || !strings.Contains(kb, `"callback_data":"menu"`) {
		t.Fatalf("клавиатура = %s", kb)
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
