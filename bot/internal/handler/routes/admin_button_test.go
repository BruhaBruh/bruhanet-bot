package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/texts"
)

const adminWebApp = `"web_app":{"url":"https://admin.example.test/tg"}`

// «🛠 Админка» — web_app в меню, только у админа; у остальных клавиатура прежняя.
func TestMenuAdminButton(t *testing.T) {
	for _, stored := range []bool{false, true} {
		for _, inPanel := range []bool{true, false} {
			e := newEnv(t)
			e.c.SetAdmin(adminID, "https://admin.example.test/")
			if stored { // та же раскладка, но из конструктора (клавиатура сообщения)
				all := map[string]texts.MsgLayout{}
				for _, key := range texts.MessagesWithKeyboard() {
					l, _, _ := texts.MessageDefault(key)
					all[key] = l
				}
				e.src.SetMessageKeyboards(all)
			}
			if inPanel {
				e.rw.AddUser(adminID, now.Add(20*day))
				e.rw.AddUser(userID, now.Add(20*day))
			}
			e.process(t, tgtest.Message(adminID, "/start"))
			kb := e.last(t, "sendMessage").Params.Get("reply_markup")
			if !strings.Contains(kb, adminWebApp) || !strings.Contains(kb, "🛠 Админка") {
				t.Errorf("stored=%v inPanel=%v: admin must see the button: %s", stored, inPanel, kb)
			}
			e.process(t, tgtest.Message(userID, "/start"))
			if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); strings.Contains(kb, "Админка") || strings.Contains(kb, "admin.example") {
				t.Errorf("stored=%v inPanel=%v: a regular user must not see the admin button: %s", stored, inPanel, kb)
			}
		}
	}
}

// Без https-адреса админки кнопки нет (Telegram не принимает http в web_app), даже у админа.
func TestMenuAdminButtonNeedsHTTPS(t *testing.T) {
	e := newEnv(t)
	e.c.SetAdmin(adminID, "http://localhost:5173")
	e.process(t, tgtest.Message(adminID, "/start"))
	if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); strings.Contains(kb, "Админка") {
		t.Fatalf("no https, no button: %s", kb)
	}
}
