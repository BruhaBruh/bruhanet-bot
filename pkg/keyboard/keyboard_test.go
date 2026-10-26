package keyboard_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/keyboard"
)

func TestInline(t *testing.T) {
	kb := keyboard.Inline(
		keyboard.Row(
			keyboard.Callback("Оплатить", "pay").Style(keyboard.StyleSuccess),
			keyboard.URL("Поддержка", "https://t.me/x"),
		),
		keyboard.Row(keyboard.Copy("Подписка", "https://sub")),
	)
	rows := kb.InlineKeyboard
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Fatalf("неверная форма клавиатуры: %+v", rows)
	}
	pay := rows[0][0]
	if pay.Text != "Оплатить" || pay.CallbackData != "pay" || pay.Style != "success" {
		t.Fatalf("callback: %+v", pay)
	}
	if support := rows[0][1]; support.URL != "https://t.me/x" || support.CallbackData != "" {
		t.Fatalf("url: %+v", support)
	}
	if sub := rows[1][0]; sub.CopyText == nil || sub.CopyText.Text != "https://sub" {
		t.Fatalf("copy: %+v", sub)
	}
}

func TestStyles(t *testing.T) {
	want := map[keyboard.Style]string{
		keyboard.StylePrimary: "primary",
		keyboard.StyleSuccess: "success",
		keyboard.StyleDanger:  "danger",
	}
	for style, value := range want {
		got := keyboard.Inline(keyboard.Row(keyboard.Callback("x", "x").Style(style))).InlineKeyboard[0][0].Style
		if got != value {
			t.Errorf("Style(%q) = %q, want %q", style, got, value)
		}
	}
}

func TestDefaultStyleOmitted(t *testing.T) {
	kb := keyboard.Inline(keyboard.Row(keyboard.Callback("x", "x").Style(keyboard.StyleDefault)))
	data, err := json.Marshal(kb)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "style") {
		t.Fatalf("пустой стиль попал в JSON: %s", data)
	}
}

func TestInlineEmpty(t *testing.T) {
	data, err := json.Marshal(keyboard.Inline())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"inline_keyboard":[]`) {
		t.Fatalf("пустая клавиатура должна сериализоваться в [], получено %s", data)
	}
}

func TestGrid(t *testing.T) {
	b := keyboard.Callback("x", "x")
	rows := keyboard.Grid(2, b, b, b)
	if len(rows) != 2 || len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Fatalf("Grid(2, 3 кнопки) = %d строк", len(rows))
	}
	if rows := keyboard.Grid(0, b, b); len(rows) != 2 {
		t.Fatalf("Grid(0) должен работать как Grid(1), получено %d строк", len(rows))
	}
	kb := keyboard.Inline(keyboard.Grid(2, b, b, b)...)
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("Inline(Grid...) = %d строк", len(kb.InlineKeyboard))
	}
}
