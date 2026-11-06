package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
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

// {{.name}} в напоминании: апдейта нет, данные — из users по id.
func TestGlobalVarInReminder(t *testing.T) {
	src := &tgtest.Source{}
	src.SetTexts(map[string]string{"reminder.day": "{{.name}}, до {{.expireAt}}"})
	src.SetContact(7, model.UserContact{FirstName: "Анна<"})
	c := content.New(src, settings.Defaults(map[int]int{1: 150}, "+7", "Банк", "https://t.me/s"), 0, nil)
	text, _ := routes.ReminderMessage(content.ForUser(t.Context(), 7), c, "day", now, "https://sub")
	if want := "Анна&lt;, до 10 января 2026 г. в 15:00, MSK"; text != want {
		t.Fatalf("got %q, want %q", text, want)
	}
}

// grant.days: дни и срок подставляются при доставке, под текстом главная клавиатура.
func TestGrantNotice(t *testing.T) {
	e := newEnv(t)
	text, kb := routes.PaymentNotice(t.Context(), e.c, e.rw, userID, "grant.days", now, 7, 0)
	if !strings.Contains(text, "Добавлено дней: <b>7</b>") || !strings.Contains(text, "10 января 2026 г. в 15:00, MSK") || kb == nil {
		t.Fatalf("text = %s kb = %v", text, kb)
	}
}
