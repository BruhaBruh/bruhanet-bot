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

func TestRenderPlain(t *testing.T) {
	got := RenderPlain("{{.months}} & {{.x}}", map[string]string{"months": "3 <м>"})
	if want := "3 <м> & x"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderDef(t *testing.T) {
	d := Def{Kind: KindMessage, Vars: []Var{{Name: "a"}, {Name: "list", Raw: true}}}
	vars := map[string]string{"a": "<i>", "list": "<code>x</code>"}
	if got, want := RenderDef(d, "{{.a}} {{.list}}", vars), "&lt;i&gt; <code>x</code>"; got != want {
		t.Fatalf("message: got %q, want %q", got, want)
	}
	for _, k := range []Kind{KindButton, KindPlain} {
		d.Kind = k
		if got, want := RenderDef(d, "{{.a}} {{.list}}", vars), "<i> <code>x</code>"; got != want {
			t.Fatalf("%s: got %q, want %q", k, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	msg := Def{Key: "t.msg", Kind: KindMessage}
	btn := Def{Key: "t.btn", Kind: KindButton}
	plain := Def{Key: "t.plain", Kind: KindPlain}
	withVar := Def{Kind: KindMessage, Vars: []Var{{Name: "name", Sample: "a&b"}}}
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
		{"href http", msg, `<a href="http://x">y</a>`, nil},
		{"href tg", msg, `<a href="tg://user?id=1">y</a>`, nil},
		{"href mailto", msg, `<a href="mailto:a@b.c">y</a>`, nil},
		{"href javascript", msg, `<a href="javascript:alert(1)">y</a>`, ErrInvalidMarkup},
		{"href JAVASCRIPT", msg, `<a href="JavaScript:alert(1)">y</a>`, ErrInvalidMarkup},
		{"href data", msg, `<a href="data:text/html,x">y</a>`, ErrInvalidMarkup},
		{"href relative", msg, `<a href="/path">y</a>`, ErrInvalidMarkup},
		{"href empty", msg, `<a href="">y</a>`, ErrInvalidMarkup},
		{"href query amp", msg, `<a href="https://x?a=1&amp;b=2">y</a>`, nil},
		{"empty message", msg, "", ErrInvalidMarkup},
		{"blank message", msg, " \n ", ErrInvalidMarkup},
		{"only tags", msg, "<b> </b>", ErrInvalidMarkup},
		{"declared var", withVar, "Привет, {{.name}}", nil},
		{"unknown var", withVar, "Привет, {{.nane}}", ErrInvalidMarkup},
		{"unknown var in button", btn, "{{.x}}", ErrBadButton},
		{"bare amp", msg, "a & b", ErrInvalidMarkup},
		{"named entity", msg, "a &nbsp; b", ErrInvalidMarkup},
		{"quot entity", msg, "&quot;x&quot;", nil},
		{"numeric entities", msg, "&#169; &#xA9; &#XA9;", nil},
		{"4096", msg, strings.Repeat("я", 4096), nil},
		{"too long", msg, strings.Repeat("я", 4097), ErrTooLong},
		{"utf16 2048 emoji", msg, strings.Repeat("😀", 2048), nil},
		{"utf16 too long", msg, strings.Repeat("😀", 2048) + "a", ErrTooLong},
		{"too long after render", Def{Kind: KindMessage, Vars: []Var{{Name: "v", Sample: strings.Repeat("a", 4000)}}}, strings.Repeat("b", 97) + "{{.v}}", ErrTooLong},
		{"too long with max values", Def{Kind: KindMessage, Vars: []Var{{Name: "v", Sample: "a", Max: 100}}}, strings.Repeat("b", 4000) + "{{.v}}", ErrTooLong},
		{"fits with max values", Def{Kind: KindMessage, Vars: []Var{{Name: "v", Sample: "a", Max: 96}}}, strings.Repeat("b", 4000) + "{{.v}}", nil},
		{"href no host", msg, `<a href="https:">y</a>`, ErrInvalidMarkup},
		{"href empty host", msg, `<a href="https:///x">y</a>`, ErrInvalidMarkup},
		{"nested blockquote", msg, "<blockquote>a<blockquote>b</blockquote></blockquote>", ErrInvalidMarkup},
		{"nested a", msg, `<a href="https://x">a<a href="https://y">b</a></a>`, ErrInvalidMarkup},
		{"tag in code", msg, "<code><b>x</b></code>", ErrInvalidMarkup},
		{"tag in pre", msg, "<pre><code>x</code></pre>", ErrInvalidMarkup},
		{"code in bold", msg, "<b><code>x</code></b>", nil},
		{"button ok", btn, "Оплатить", nil},
		{"button empty", btn, "", ErrBadButton},
		{"button blank", btn, "  ", ErrBadButton},
		{"button tag", btn, "<b>x</b>", ErrBadButton},
		{"button 64", btn, strings.Repeat("я", 64), nil},
		{"button 65", btn, strings.Repeat("я", 65), ErrBadButton},
		{"button utf16 65", btn, strings.Repeat("😀", 32) + "a", ErrBadButton},
		{"button bare amp ok", btn, "Купить & продлить", nil},
		{"plain ok", plain, "Активна", nil},
		{"plain empty", plain, " ", ErrBadButton},
		{"plain tag", plain, "<i>x</i>", ErrBadButton},
		{"plain 65", plain, strings.Repeat("a", 65), ErrBadButton},
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
		if d.Title == "" || (d.Kind != KindMessage && d.Kind != KindButton && d.Kind != KindPlain) {
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
			if v.Max < utf16Len(v.Sample) {
				t.Errorf("%s: var %s Max %d < sample", d.Key, v.Name, v.Max)
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
		if got := RenderDef(d, d.Default, Samples(d)); got != want {
			t.Errorf("%s:\ngot  %q\nwant %q", key, got, want)
		}
	}
}

func TestStatusesArePlain(t *testing.T) {
	for _, k := range []string{"menu.status_active", "menu.status_disabled", "menu.status_limited", "menu.status_expired", "menu.status_unknown"} {
		if d, ok := Lookup(k); !ok || d.Kind != KindPlain {
			t.Errorf("%s: want KindPlain", k)
		}
	}
}

func TestRegistryDeepClone(t *testing.T) {
	r := Registry()
	i := slices.IndexFunc(r, func(d Def) bool { return len(d.Vars) > 0 })
	key, name := r[i].Key, r[i].Vars[0].Name
	r[i].Vars[0].Name = "mutated"
	r[i].Default = "mutated"
	d, _ := Lookup(key)
	if d.Vars[0].Name != name || d.Default == "mutated" {
		t.Fatal("Registry() leaked the global registry")
	}
	d.Vars[0].Name = "mutated"
	if d2, _ := Lookup(key); d2.Vars[0].Name != name {
		t.Fatal("Lookup() leaked the global registry")
	}
}

func TestProblem(t *testing.T) {
	def, _ := Lookup("menu.greeting")
	err := Validate(def, "<b>x")
	if err == nil {
		t.Fatal("want error")
	}
	got := Problem(err)
	if strings.Contains(got, "invalid telegram html") || !strings.Contains(got, "не закрыт") {
		t.Fatalf("problem = %q", got)
	}
}

func TestVisibleLen(t *testing.T) {
	if n := VisibleLen("<b>я&amp;</b>😀"); n != 4 {
		t.Fatalf("n = %d", n)
	}
}
