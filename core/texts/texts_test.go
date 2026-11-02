package texts

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRenderEscapes(t *testing.T) {
	got := Render("Привет, {{.name}}", map[string]string{"name": "a<b>&"})
	if want := "Привет, a&lt;b&gt;&amp;"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderUnknown(t *testing.T) {
	got := Render("{{.foo.bar}} и {{ .x }}", nil)
	if want := "foo.bar и x"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderRaw(t *testing.T) {
	got := RenderRaw("{{.a}} {{.b}}", map[string]string{"a": "<i>"}, map[string]string{"b": "<code>x</code>"})
	if want := "&lt;i&gt; <code>x</code>"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	msg := Def{Key: "t.msg", Kind: KindMessage}
	btn := Def{Key: "t.btn", Kind: KindButton}
	tests := []struct {
		name string
		def  Def
		body string
		want error
	}{
		{"bold", msg, "<b>ok</b>", nil},
		{"unclosed", msg, "<b>bad", ErrInvalidMarkup},
		{"mismatched", msg, "<b><i>x</b></i>", ErrInvalidMarkup},
		{"script", msg, "<script>x</script>", ErrInvalidMarkup},
		{"link", msg, `<a href="https://x">y</a>`, nil},
		{"onclick", msg, `<a onclick="x">y</a>`, ErrInvalidMarkup},
		{"spoiler span", msg, `<span class="tg-spoiler">x</span>`, nil},
		{"plain span", msg, `<span>x</span>`, ErrInvalidMarkup},
		{"bare lt", msg, "a < b", ErrInvalidMarkup},
		{"entities", msg, "a &lt; b &amp; c", nil},
		{"4096", msg, strings.Repeat("я", 4096), nil},
		{"too long", msg, strings.Repeat("я", 4097), ErrTooLong},
		{"too long after render", Def{Kind: KindMessage, Vars: []Var{{Name: "v", Sample: strings.Repeat("a", 4000)}}}, strings.Repeat("b", 97) + "{{.v}}", ErrTooLong},
		{"button ok", btn, "Оплатить", nil},
		{"button empty", btn, "", ErrBadButton},
		{"button blank", btn, "  ", ErrBadButton},
		{"button tag", btn, "<b>x</b>", ErrBadButton},
		{"button 64", btn, strings.Repeat("я", 64), nil},
		{"button 65", btn, strings.Repeat("я", 65), ErrBadButton},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.def, tt.body)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRegistryDefaultsValid(t *testing.T) {
	groups := []string{"Меню", "Оплата", "Напоминания", "Ошибки", "Кнопки"}
	seen := map[string]bool{}
	lastGroup := 0
	for _, d := range Registry() {
		if seen[d.Key] {
			t.Errorf("duplicate key %s", d.Key)
		}
		seen[d.Key] = true
		g := slices.Index(groups, d.Group)
		if g < lastGroup {
			t.Errorf("%s: group %q out of order", d.Key, d.Group)
		}
		lastGroup = g
		if d.Title == "" || (d.Kind != KindMessage && d.Kind != KindButton) {
			t.Errorf("%s: empty title or bad kind", d.Key)
		}
		if err := Validate(d, d.Default); err != nil {
			t.Errorf("%s: default invalid: %v", d.Key, err)
		}
		used := map[string]bool{}
		for _, m := range placeholder.FindAllStringSubmatch(d.Default, -1) {
			used[m[1]] = true
		}
		for _, v := range d.Vars {
			if !used[v.Name] {
				t.Errorf("%s: var %s not used in default", d.Key, v.Name)
			}
			if v.Sample == "" || v.Description == "" {
				t.Errorf("%s: var %s without sample or description", d.Key, v.Name)
			}
			delete(used, v.Name)
		}
		for name := range used {
			t.Errorf("%s: placeholder %s has no Var", d.Key, name)
		}
		if got, ok := Lookup(d.Key); !ok || got.Key != d.Key {
			t.Errorf("%s: lookup failed", d.Key)
		}
	}
	if _, ok := Lookup("no.such"); ok {
		t.Error("lookup of unknown key succeeded")
	}
}

func TestRegistryCoversCode(t *testing.T) {
	tests := map[string]string{
		"menu.greeting": "Здравствуйте, <b>Иван</b>",
		"payment.requisites": "<b>Оплата тарифа на 3 месяца за 500 ₽</b>\n" +
			"\n" +
			"<b>Для оплаты</b>\n" +
			"Переведите <b>500 ₽</b>\n" +
			"на номер <code>+79990000000</code> в <b>Т-Банк</b>\n" +
			"с одним из комментариев:\n" +
			"- <code>На кофе</code>\n" +
			"- <code>Перекус</code>\n" +
			"- <code>Одолжил</code>\n" +
			"- <code>Вернул долг</code>\n" +
			"и отправьте в чат фото платежа",
		"reminder.day": "<b>Уведомление об окончании подписки</b>\n" +
			"\n" +
			"Ваша подписка закончится в течение суток\n" +
			"\n" +
			"<b>Дата окончания подписки</b>\n" +
			"2 ноября 2026 г. в 15:04, MSK",
	}
	for key, want := range tests {
		d, ok := Lookup(key)
		if !ok {
			t.Fatalf("no %s", key)
		}
		if got := renderSamples(d, d.Default); got != want {
			t.Errorf("%s:\ngot  %q\nwant %q", key, got, want)
		}
	}
}
