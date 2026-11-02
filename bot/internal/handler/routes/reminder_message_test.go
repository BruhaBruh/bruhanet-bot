package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/settings"
)

func TestReminderMessage(t *testing.T) {
	const sub = "https://sub.example/abc"
	c := content.New(&tgtest.Source{}, settings.Defaults(map[int]int{1: 150}, "+7", "Банк", "https://t.me/s"), 0, nil)
	for kind, want := range map[string]string{"day": "в течение суток", "week": "скоро закончится"} {
		text, kb := routes.ReminderMessage(t.Context(), c, kind, now, sub)
		if !strings.Contains(text, want) || !strings.Contains(text, "<b>Дата окончания подписки</b>\n10 января 2026 г. в 15:00, MSK") {
			t.Fatalf("%s: text = %s", kind, text)
		}
		if len(kb.InlineKeyboard) != 1 || kb.InlineKeyboard[0][0].CallbackData != "pay" || kb.InlineKeyboard[0][1].CopyText.Text != sub {
			t.Fatalf("%s: kb = %+v", kind, kb)
		}
	}
}
