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
		len(g.Vars) != 1 || g.Vars[0].Name != "name" || g.Vars[0].Sample == "" {
		t.Fatalf("greeting = %+v", g)
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
