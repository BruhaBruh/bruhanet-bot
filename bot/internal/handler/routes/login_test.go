package routes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func TestLoginAdmin(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(adminID, "/login"))

	toks := e.pay.LoginTokens()
	if len(toks) != 1 || !toks[0].ExpiresAt.Equal(now.Add(5*time.Minute)) || len(toks[0].Hash) != 32 {
		t.Fatalf("tokens = %+v", toks)
	}
	p := e.last(t, "sendMessage").Params
	if p.Get("chat_id") != "1" || !strings.Contains(p.Get("text"), "Вход в админку") ||
		!strings.Contains(p.Get("text"), "5 минут") {
		t.Fatalf("params = %v", p)
	}
	if kb := p.Get("reply_markup"); !strings.Contains(kb, `"url":"http://localhost:5173/auth?token=`) {
		t.Fatalf("reply_markup = %s", kb)
	}
	// превью-краулер Telegram сжёг бы одноразовый токен: ссылки нет в тексте, превью выключено
	if text := p.Get("text"); strings.Contains(text, "auth") {
		t.Fatalf("token leaked into text: %s", text)
	}
	if lp := p.Get("link_preview_options"); !strings.Contains(lp, `"is_disabled":true`) {
		t.Fatalf("link_preview_options = %q", lp)
	}
}

func TestLoginNonAdmin(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(userID, "/login"))
	if n := len(e.pay.LoginTokens()); n != 0 {
		t.Fatalf("tokens = %d", n)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); strings.Contains(text, "Вход в админку") {
		t.Fatalf("text = %s", text)
	}
	if e.last(t, "sendMessage").Params.Get("chat_id") != "42" {
		t.Fatal("reply must go to the user")
	}
}
