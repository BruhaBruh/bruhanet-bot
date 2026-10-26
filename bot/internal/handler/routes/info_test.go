package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func TestInfoActiveUser(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC))
	e.process(t, tgtest.Callback(userID, "info"))
	p := e.last(t, "editMessageText").Params
	text := p.Get("text")
	for _, want := range []string{"Активна", "0 Б из ∞", `10 февраля 2026 г\. в 15:00, MSK`, "42"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text без %q: %s", want, text)
		}
	}
	kb := p.Get("reply_markup")
	for _, want := range []string{`"callback_data":"pay"`, `"callback_data":"menu"`, `"copy_text"`} {
		if !strings.Contains(kb, want) {
			t.Fatalf("клавиатура без %s: %s", want, kb)
		}
	}
}

func TestInfoWithoutUser(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "info"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Не куплена") {
		t.Fatalf("text = %s", text)
	}
}

func TestInfoRemnawaveDown(t *testing.T) {
	e := newEnv(t)
	e.rw.FailWith(errors.New("panel is down"))
	e.process(t, tgtest.Callback(userID, "info"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("text = %s", text)
	}
	tgtest.FindLog(t, e.log, "failed to get remnawave user")
}
