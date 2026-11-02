package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
)

func TestReminderMessage(t *testing.T) {
	const sub = "https://sub.example/abc"
	for kind, want := range map[string]string{"day": "в течение суток", "week": "скоро закончится"} {
		text, kb := routes.ReminderMessage(kind, now, sub)
		if !strings.Contains(text, want) {
			t.Fatalf("%s: text = %s", kind, text)
		}
		if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].CallbackData != "pay" || kb.InlineKeyboard[0][1].CopyText.Text != sub {
			t.Fatalf("%s: kb = %+v", kind, kb)
		}
	}
}
