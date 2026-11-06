package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/texts"
)

func send(h http.Handler, method, target, cookie, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestTextsList(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.overrides = map[string]string{"menu.greeting": "Привет, <b>{{.name}}</b>"}
	st.setRaw = map[string][]byte{"vars": []byte(`{"promoLink":"https://x.io"}`)}

	w := send(h, http.MethodGet, "/api/texts", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out struct {
		Items []struct {
			Key, Group, Title, Kind, Default, Body string
			Overridden                             bool
			Vars                                   []struct{ Name, Description, Sample string }
		}
		Globals []struct {
			Name, Description, Sample string
			Max                       int
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	reg := texts.Registry()
	if len(out.Items) != len(reg) {
		t.Fatalf("items = %d, registry = %d", len(out.Items), len(reg))
	}
	for i, it := range out.Items {
		if it.Key != reg[i].Key || it.Default != reg[i].Default {
			t.Fatalf("order/default mismatch at %d: %s", i, it.Key)
		}
	}
	g := out.Items[0]
	if g.Key != "menu.greeting" || !g.Overridden || g.Body != "Привет, <b>{{.name}}</b>" || g.Kind != "message" ||
		len(g.Vars) != 0 {
		t.Fatalf("greeting = %+v", g)
	}
	names := map[string]string{}
	for _, v := range out.Globals {
		if v.Description == "" || v.Sample == "" || v.Max == 0 {
			t.Fatalf("global = %+v", v)
		}
		names[v.Name] = v.Sample
	}
	for _, n := range []string{"name", "username", "telegramId", "expireAt", "phone", "bank", "supportUrl"} {
		if names[n] == "" {
			t.Fatalf("no global %s in %v", n, names)
		}
	}
	if names["promoLink"] != "https://x.io" {
		t.Fatalf("custom global = %v", names)
	}
	if o := out.Items[1]; o.Overridden || o.Body != o.Default {
		t.Fatalf("second = %+v", o)
	}
	if !strings.Contains(w.Body.String(), `"name":"comments","description":`) ||
		!strings.Contains(w.Body.String(), `"raw":true`) { // comments — готовый HTML, превью вставляет его как есть
		t.Fatal("comments var must be marked raw")
	}
	if !strings.Contains(w.Body.String(), `"vars":[]`) { // без переменных — [], не null
		t.Fatal("vars must be [] for texts without variables")
	}
}

// Review Focus 1: битая разметка → 400 с человеческим message, без служебного префикса сентинела.
func TestTextsWhenAndButtons(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.styles = map[string]string{"button.pay": "danger"}
	w := send(h, http.MethodGet, "/api/texts", c, "")
	var out struct {
		Items []struct {
			Key, When, Route, Style string
			Buttons                 []string
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	by := map[string]int{}
	for i, it := range out.Items {
		if it.When == "" {
			t.Errorf("%s: no when", it.Key)
		}
		by[it.Key] = i
	}
	if o := out.Items[by["payment.confirm"]]; strings.Join(o.Buttons, ",") != "button.yes,button.no,button.have_promo" {
		t.Fatalf("buttons = %+v", o)
	}
	if o := out.Items[by["button.pay"]]; o.Style != "danger" || o.Route != "pay" {
		t.Fatalf("stored style = %+v", o)
	}
	if o := out.Items[by["button.buy"]]; o.Style != "success" {
		t.Fatalf("default style = %+v", o)
	}
	if !strings.Contains(w.Body.String(), `"buttons":[]`) {
		t.Fatal("buttons must be [] when none")
	}
}

func TestPutTextStyle(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	put := func(key, body string) int { return send(h, http.MethodPut, "/api/texts/"+key, c, body).Code }

	if code := put("button.pay", `{"body":"Оплатить","style":"danger"}`); code != http.StatusNoContent {
		t.Fatalf("%d", code)
	}
	if st.styles["button.pay"] != "danger" || len(st.overrides) != 0 {
		t.Fatalf("styles %v overrides %v", st.styles, st.overrides)
	}
	// подпись без style стиль не трогает; подпись = умолчание не сбрасывает стиль
	if code := put("button.pay", `{"body":"Платить"}`); code != http.StatusNoContent || st.styles["button.pay"] != "danger" || st.overrides["button.pay"] != "Платить" {
		t.Fatalf("%d %v %v", code, st.styles, st.overrides)
	}
	if code := put("button.pay", `{"body":"Оплатить"}`); code != http.StatusNoContent || st.styles["button.pay"] != "danger" || len(st.overrides) != 0 {
		t.Fatalf("%d %v %v", code, st.styles, st.overrides)
	}
	// «Обычная» при основном виде success хранится явно; вид по умолчанию — снимается
	if code := put("button.buy", `{"body":"Купить","style":""}`); code != http.StatusNoContent {
		t.Fatalf("%d", code)
	}
	if v, ok := st.styles["button.buy"]; !ok || v != "" {
		t.Fatalf("styles = %v", st.styles)
	}
	if code := put("button.buy", `{"body":"Купить","style":"success"}`); code != http.StatusNoContent {
		t.Fatalf("%d", code)
	}
	if _, ok := st.styles["button.buy"]; ok {
		t.Fatalf("styles = %v", st.styles)
	}
	// чужой стиль и стиль не у кнопки
	if code := put("button.pay", `{"body":"Оплатить","style":"pink"}`); code != http.StatusBadRequest {
		t.Fatalf("%d", code)
	}
	if code := put("payment.tariffs", `{"body":"x","style":"danger"}`); code != http.StatusBadRequest {
		t.Fatalf("%d", code)
	}
}

func TestPutTextGlobals(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"a"}`)}
	for key, body := range map[string]string{
		"payment.tariffs":    "{{.name}} {{.my}} {{.expireAt}}",
		"button.pay":         "Оплатить, {{.username}}", // имя до 64 символов в кнопку не влезло бы
		"menu.status_active": "{{.my}}",
	} {
		b, _ := json.Marshal(map[string]string{"body": body})
		if w := send(h, http.MethodPut, "/api/texts/"+key, c, string(b)); w.Code != http.StatusNoContent {
			t.Errorf("%s: %d %s", key, w.Code, w.Body)
		}
	}
	if w := send(h, http.MethodPut, "/api/texts/payment.tariffs", c, `{"body":"{{.nope}}"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := send(h, http.MethodPut, "/api/texts/payment.tariffs", c, `{"body":"{{name}}"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "неправильная переменная") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestPutTextInvalid(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	for _, tc := range []struct{ body, want string }{
		{"<b>x", "не закрыт"},
		{"<script>x</script>", "не поддерживается"},
		{"{{.nope}}", "неизвестная переменная nope"},
		{"", "пустым"},
	} {
		b, _ := json.Marshal(map[string]string{"body": tc.body})
		w := send(h, http.MethodPut, "/api/texts/menu.greeting", c, string(b))
		var out map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != http.StatusBadRequest || out["error"] != "invalid" || !strings.Contains(out["message"], tc.want) ||
			strings.Contains(out["message"], "invalid telegram html") {
			t.Errorf("%q: %d %s", tc.body, w.Code, w.Body)
		}
	}
	if len(st.overrides) != 0 {
		t.Fatalf("invalid body saved: %v", st.overrides)
	}
	if w := send(h, http.MethodPut, "/api/texts/menu.greeting", c, `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("no body field: %d", w.Code)
	}
	if w := send(h, http.MethodPut, "/api/texts/menu.greeting", c, `{`); w.Code != http.StatusBadRequest {
		t.Fatalf("broken json: %d", w.Code)
	}
}

func TestTextUnknownKey(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	for _, r := range []struct{ method, path string }{
		{http.MethodPut, "/api/texts/nope"},
		{http.MethodDelete, "/api/texts/nope"},
		{http.MethodPost, "/api/texts/nope/test"},
	} {
		if w := send(h, r.method, r.path, c, `{"body":"x"}`); w.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d", r.method, r.path, w.Code)
		}
	}
}

func TestPutTextSaveAndReset(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	def, _ := texts.Lookup("menu.greeting")

	if w := send(h, http.MethodPut, "/api/texts/menu.greeting", c, `{"body":"Хай, <i>{{.name}}</i>"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if st.overrides["menu.greeting"] != "Хай, <i>{{.name}}</i>" {
		t.Fatalf("overrides = %v", st.overrides)
	}
	// тело, равное дефолту, — сброс, а не копия дефолта в БД
	b, _ := json.Marshal(map[string]string{"body": def.Default})
	if w := send(h, http.MethodPut, "/api/texts/menu.greeting", c, string(b)); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, ok := st.overrides["menu.greeting"]; ok {
		t.Fatalf("default body stored as override: %v", st.overrides)
	}

	st.overrides = map[string]string{"menu.greeting": "x", "button.pay": "y"}
	if w := send(h, http.MethodDelete, "/api/texts/menu.greeting", c, ""); w.Code != http.StatusNoContent {
		t.Fatalf("%d", w.Code)
	}
	if len(st.overrides) != 1 || st.overrides["button.pay"] != "y" {
		t.Fatalf("overrides = %v", st.overrides)
	}
}

func TestTextTest(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	if w := send(h, http.MethodPost, "/api/texts/menu.greeting/test", c, `{"body":"Хай, <b>{{.name}}</b>"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(st.outbox) != 1 {
		t.Fatalf("outbox = %+v", st.outbox)
	}
	m := st.outbox[0]
	if m.EventType != "text_test" || m.ChatID != adminID || m.ParseMode != "HTML" || m.Text != "Хай, <b>Иван</b>" {
		t.Fatalf("msg = %+v", m)
	}
	if len(st.overrides) != 0 {
		t.Fatal("test must not save")
	}

	// без тела — текущий (override); кнопка → «Кнопка: …»
	st.overrides = map[string]string{"button.pay": "Платить"}
	if w := send(h, http.MethodPost, "/api/texts/button.pay/test", c, ""); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if m := st.outbox[1]; m.Text != "Кнопка: Платить" {
		t.Fatalf("msg = %+v", m)
	}

	// битое тело — 400, в outbox ничего
	w := send(h, http.MethodPost, "/api/texts/menu.greeting/test", c, `{"body":"<b>x"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error":"invalid"`) || len(st.outbox) != 2 {
		t.Fatalf("%d %s outbox=%d", w.Code, w.Body, len(st.outbox))
	}
}

func TestTextBodyTooLong(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	big := `{"body":"` + strings.Repeat("a", 17<<10) + `"}`
	for _, target := range []string{"/api/texts/menu.greeting", "/api/texts/menu.greeting/test"} {
		method := http.MethodPut
		if strings.HasSuffix(target, "/test") {
			method = http.MethodPost
		}
		w := send(h, method, target, c, big)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "слишком длинный текст") {
			t.Errorf("%s: %d %s", target, w.Code, w.Body)
		}
	}
}

// Кнопка и простой текст: переменная, которая может не влезть в 64 символа, — отказ при сохранении.
func TestPutTextButtonVarsMustFit(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"` + strings.Repeat("я", 61) + `"}`)}
	put := func(key, body string) *httptest.ResponseRecorder {
		return send(h, http.MethodPut, "/api/texts/"+key, c, `{"body":"`+body+`"}`)
	}
	for _, key := range []string{"button.pay", "menu.status_active"} {
		w := put(key, "Привет, {{.name}}")
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "{{.name}} может не влезть") || !strings.Contains(w.Body.String(), "до 64 символов") {
			t.Errorf("%s name: %d %s", key, w.Code, w.Body)
		}
		if w := put(key, "{{.name}}"); w.Code != http.StatusNoContent { // ровно 64
			t.Errorf("%s 64: %d %s", key, w.Code, w.Body)
		}
		if w := put(key, "ID {{.telegramId}}"); w.Code != http.StatusNoContent {
			t.Errorf("%s short: %d %s", key, w.Code, w.Body)
		}
		// своя переменная — по текущему значению (61): 3 + 61 = 64 проходит, 4 + 61 — нет
		if w := put(key, "ab {{.my}}"); w.Code != http.StatusNoContent {
			t.Errorf("%s custom 64: %d %s", key, w.Code, w.Body)
		}
		if w := put(key, "abc {{.my}}"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "{{.my}}") {
			t.Errorf("%s custom 65: %d %s", key, w.Code, w.Body)
		}
	}
	// сообщение этого лимита не знает
	if w := put("menu.greeting", "Привет, {{.name}} {{.my}} {{.supportUrl}}"); w.Code != http.StatusNoContent {
		t.Errorf("message: %d %s", w.Code, w.Body)
	}
}
