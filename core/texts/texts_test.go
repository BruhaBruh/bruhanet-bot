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
	if want := " и "; got != want { // неизвестная переменная не ломает текст
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
	if want := "3 <м> & "; got != want {
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
			err := Validate(tt.def, tt.body, nil)
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
	groups := []string{"Меню", "Оплата", "Промокоды", "Напоминания", "Ошибки", "Кнопки"}
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
		if err := Validate(d, d.Default, nil); err != nil {
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
		for _, v := range Globals(nil) {
			if slices.ContainsFunc(d.Vars, func(x Var) bool { return x.Name == v.Name }) {
				t.Errorf("%s: var %s collides with a global", d.Key, v.Name)
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

func TestRegistryDescribesEveryText(t *testing.T) {
	for _, d := range Registry() {
		if d.When == "" {
			t.Errorf("%s: empty When", d.Key)
		}
		for _, k := range d.Buttons {
			if b, ok := Lookup(k); !ok || b.Kind != KindButton {
				t.Errorf("%s: %s is not a button", d.Key, k)
			}
		}
		if (d.Kind == KindButton) != (d.Route != "") {
			t.Errorf("%s: route %q for kind %s", d.Key, d.Route, d.Kind)
		}
		if d.Kind != KindButton && (d.Style != "" || d.Route != "") {
			t.Errorf("%s: style or route on non-button", d.Key)
		}
		if !slices.Contains(Styles, d.Style) {
			t.Errorf("%s: bad style %q", d.Key, d.Style)
		}
		if d.Kind != KindMessage && len(d.Buttons) > 0 {
			t.Errorf("%s: buttons on non-message", d.Key)
		}
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
		if got := RenderDef(d, d.Default, Samples(d, nil)); got != want {
			t.Errorf("%s:\ngot  %q\nwant %q", key, got, want)
		}
	}
}

func TestGlobalsValidEverywhere(t *testing.T) {
	custom := map[string]string{"promoLink": "https://x.io/a"}
	for _, d := range Registry() {
		for _, name := range []string{"name", "username", "telegramId", "expireAt", "phone", "bank", "supportUrl", "promoLink"} {
			err := Validate(d, "a {{."+name+"}}", custom)
			// кнопки и простые тексты: длинные по худшему случаю переменные не проходят
			tooLong := d.Kind != KindMessage && (name == "name" || name == "phone" || name == "bank" || name == "supportUrl")
			if tooLong != (err != nil) {
				t.Errorf("%s with %s: %v (too long: %v)", d.Key, name, err, tooLong)
			}
		}
	}
	for _, v := range Globals(nil) {
		if v.Sample == "" || v.Description == "" || v.Max < utf16Len(v.Sample) {
			t.Errorf("global %s: %+v", v.Name, v)
		}
		if !Reserved(v.Name) {
			t.Errorf("%s not reserved", v.Name)
		}
	}
	if !Reserved("months") || Reserved("promoLink") {
		t.Fatal("Reserved: registry var must be reserved, custom name must not")
	}
}

// Значение с < не ломает HTML: экранируется и общая, и своя переменная.
func TestGlobalValueEscaped(t *testing.T) {
	d, _ := Lookup("menu.greeting")
	got := RenderDef(d, "{{.name}} {{.my}}", map[string]string{"name": "<b>a&", "my": "1<2"})
	if want := "&lt;b&gt;a&amp; 1&lt;2"; got != want {
		t.Fatalf("got %q", got)
	}
	if _, err := checkMarkup(got); err != nil {
		t.Fatal(err)
	}
}

func TestWorstCaseCountsGlobals(t *testing.T) {
	d := Def{Kind: KindMessage}
	// name 64 + 4032 = 4096 — помещается, на символ больше — нет
	if err := Validate(d, strings.Repeat("b", 4032)+"{{.name}}", nil); err != nil {
		t.Fatal(err)
	}
	if err := Validate(d, strings.Repeat("b", 4033)+"{{.name}}", nil); !errors.Is(err, ErrTooLong) {
		t.Fatalf("err = %v", err)
	}
	custom := map[string]string{"my": "a"}
	if err := Validate(d, strings.Repeat("b", 4096-MaxCustomVar+1)+"{{.my}}", custom); !errors.Is(err, ErrTooLong) {
		t.Fatalf("custom: err = %v", err)
	}
	if n := WorstLen(d, "ab{{.name}}", nil); n != 66 {
		t.Fatalf("WorstLen = %d", n)
	}
}

// Ссылка из переменной в href: и пример, и худший случай проходят проверку схемы.
func TestSupportURLInHref(t *testing.T) {
	if err := Validate(Def{Kind: KindMessage}, `<a href="{{.supportUrl}}">помощь</a>`, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedVar(t *testing.T) {
	d := Def{Kind: KindMessage}
	for _, body := range []string{"{{name}}", "{{.na me}}", "{{.name}", "{{.name", "{{ .name } }"} {
		if err := Validate(d, body, nil); !errors.Is(err, ErrInvalidMarkup) || !strings.Contains(err.Error(), "неправильная переменная") {
			t.Errorf("%q: %v", body, err)
		}
	}
	if err := Validate(d, "{{.na<b>me</b>}}", nil); !errors.Is(err, ErrInvalidMarkup) { // разметка внутри переменной
		t.Errorf("markup inside var: %v", err)
	}
	for _, body := range []string{"a {{ b", "{ {.x}}", "{{.name}} }}"} { // обычный текст
		if err := Validate(d, body, nil); err != nil {
			t.Errorf("%q: %v", body, err)
		}
	}
	if err := Validate(Def{Kind: KindButton}, "{{name}}", nil); !errors.Is(err, ErrBadButton) {
		t.Fatalf("button: %v", err)
	}
}

func TestValidateStoredKeepsUnknownVar(t *testing.T) {
	d, _ := Lookup("menu.greeting")
	if err := Validate(d, "a {{.gone}}", nil); err == nil {
		t.Fatal("Validate must refuse unknown var")
	}
	if err := ValidateStored(d, "a {{.gone}}", nil); err != nil {
		t.Fatal(err)
	}
	if got := RenderDef(d, "a {{.gone}}", nil); got != "a " {
		t.Fatalf("render = %q", got)
	}
	if err := ValidateStored(d, "<b>a", nil); !errors.Is(err, ErrInvalidMarkup) {
		t.Fatalf("broken markup: %v", err)
	}
}

func TestButtonVarsWorstCase(t *testing.T) {
	d, _ := Lookup("button.pay")
	pl, _ := Lookup("menu.status_active")
	for _, def := range []Def{d, pl} {
		// имя до 64: одна переменная ровно в лимит проходит, +1 символ — нет
		if err := Validate(def, "{{.name}}", nil); err != nil {
			t.Fatalf("%s: %v", def.Key, err)
		}
		err := Validate(def, "!{{.name}}", nil)
		if !errors.Is(err, ErrBadButton) || !strings.Contains(Problem(err), "{{.name}} может не влезть") || !strings.Contains(Problem(err), "до 64 символов") {
			t.Fatalf("%s: %v", def.Key, err)
		}
		// короткие переменные проходят
		if err := Validate(def, "{{.telegramId}} до {{.expireAt}}", nil); err != nil {
			t.Fatalf("%s short: %v", def.Key, err)
		}
	}
	// называется самая длинная переменная
	if err := Validate(d, "{{.telegramId}} {{.supportUrl}}", nil); !strings.Contains(Problem(err), "{{.supportUrl}}") {
		t.Fatalf("widest: %v", err)
	}
	// переменные кнопки: months(16)+price(8) помещаются, длинный хвост — нет
	tariff, _ := Lookup("button.tariff")
	if err := Validate(tariff, tariff.Default, nil); err != nil {
		t.Fatal(err)
	}
	if err := Validate(tariff, tariff.Default+strings.Repeat("я", 40), nil); !errors.Is(err, ErrBadButton) { // 30+40 = 70
		t.Fatalf("tail: %v", err)
	}
	if err := Validate(tariff, tariff.Default+strings.Repeat("я", 34), nil); err != nil { // 30+34 = 64
		t.Fatalf("64: %v", err)
	}
	if err := Validate(tariff, tariff.Default+strings.Repeat("я", 35), nil); !errors.Is(err, ErrBadButton) { // 65
		t.Fatalf("65: %v", err)
	}
}

func TestButtonCustomVarByCurrentValue(t *testing.T) {
	d, _ := Lookup("button.pay")
	at := func(n int) map[string]string { return map[string]string{"my": strings.Repeat("я", n)} }
	if err := Validate(d, "{{.my}}", at(64)); err != nil {
		t.Fatalf("64: %v", err)
	}
	err := Validate(d, "{{.my}}", at(65))
	if !errors.Is(err, ErrBadButton) || !strings.Contains(Problem(err), "{{.my}} может не влезть в кнопку") {
		t.Fatalf("65: %v", err)
	}
	// эмодзи — 2 UTF-16 units
	if err := Validate(d, "{{.my}}", map[string]string{"my": strings.Repeat("😀", 33)}); !errors.Is(err, ErrBadButton) {
		t.Fatalf("emoji 66: %v", err)
	}
	// значение с < в кнопке — просто текст, правку подписи не отменяет
	if err := Validate(d, "{{.my}}", map[string]string{"my": "a<b"}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderPlainNotClipped(t *testing.T) {
	d, _ := Lookup("button.pay")
	long := strings.Repeat("я", 70)
	if got := RenderDef(d, "{{.x}}", map[string]string{"x": long}); got != long || FitsPlain(got) {
		t.Fatalf("render must not clip: %d", utf16Len(got))
	}
	if !FitsPlain(strings.Repeat("я", 64)) || FitsPlain(strings.Repeat("я", 65)) || FitsPlain(" ") || FitsPlain("") {
		t.Fatal("FitsPlain boundary")
	}
	if n := utf16Len(ClipPlain(long)); n != 64 {
		t.Fatalf("ClipPlain = %d", n)
	}
}

func TestBlankAndUses(t *testing.T) {
	if !Blank("") || !Blank(" <b> </b>\n") || Blank("<b>a</b>") || Blank("a<b") {
		t.Fatal("Blank")
	}
	if !Uses("x {{ .name }}", "name") || Uses("{{.names}}", "name") || !HasVars("{{.a}}") || HasVars("{{ a") {
		t.Fatal("Uses/HasVars")
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
	err := Validate(def, "<b>x", nil)
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
