package server_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/texts"
)

type kbOut struct {
	ID         string
	Messages   []string
	Rows       [][]string
	Hidden     []string
	Default    [][]string
	Overridden bool
	Inactive   bool
	Problem    string
	Slots      []struct {
		ID, Kind, Label, Style string
		Removable              bool
	}
	Dynamic []struct{ ID, Title string }
}

func listKeyboards(t *testing.T, h http.Handler, c string) map[string][]kbOut {
	t.Helper()
	w := send(h, http.MethodGet, "/api/texts", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out struct {
		Items []struct {
			Key       string
			Keyboards []kbOut
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	m := map[string][]kbOut{}
	for _, it := range out.Items {
		if it.Keyboards == nil {
			t.Fatalf("%s: keyboards must be [] not null", it.Key)
		}
		m[it.Key] = it.Keyboards
	}
	return m
}

func TestTextsKeyboardsShape(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.overrides = map[string]string{"button.pay": "Заплатить"}
	st.styles = map[string]string{"button.pay": "success"}
	kbs := listKeyboards(t, h, c)

	if len(kbs["menu.info_not_bought"]) != 1 || len(kbs["menu.status_active"]) != 0 {
		t.Fatalf("keyboards: %v", kbs["menu.info_not_bought"])
	}
	// главное меню — две клавиатуры (юзер в панели и нет), общие для десятка сообщений
	if g := kbs["menu.greeting"]; len(g) != 2 || g[0].ID != "menu" || g[1].ID != "menu_new" {
		t.Fatalf("greeting keyboards: %+v", g)
	}
	menu := kbs["menu.greeting"][0]
	if !slices.EqualFunc(menu.Rows, menu.Default, slices.Equal) || menu.Overridden || len(menu.Hidden) != 0 || !slices.Contains(menu.Messages, "payment.confirmed") {
		t.Fatalf("menu: %+v", menu)
	}
	var pay struct {
		Label, Style string
		Removable    bool
	}
	for _, s := range menu.Slots {
		if s.ID == "button.pay" {
			pay.Label, pay.Style, pay.Removable = s.Label, s.Style, s.Removable
		}
	}
	if pay.Label != "Заплатить" || pay.Style != "success" || !pay.Removable {
		t.Fatalf("pay slot: %+v", pay)
	}
	tariffs := kbs["payment.tariffs"][0]
	if len(tariffs.Dynamic) != 1 || tariffs.Dynamic[0].ID != "@tariffs" {
		t.Fatalf("tariffs: %+v", tariffs)
	}
	for _, s := range tariffs.Slots {
		if (s.ID == "@tariffs") != (s.Kind == "dynamic") || s.Removable {
			t.Fatalf("slot %+v", s)
		}
	}
	confirm := kbs["payment.confirm"][0]
	for _, s := range confirm.Slots {
		if want := s.ID == "button.have_promo"; s.Removable != want {
			t.Fatalf("%s removable = %v", s.ID, s.Removable)
		}
	}
}

func TestPutKeyboard(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	body := `{"rows":[["button.pay","button.info"],["button.gift"]],"hidden":["button.referral","button.promo"]}`
	if w := send(h, http.MethodPut, "/api/keyboards/menu", c, body); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	l := st.layouts["menu"]
	if len(l.Rows) != 2 || l.Rows[0][0] != "button.pay" || len(l.Hidden) != 2 {
		t.Fatalf("stored: %+v", l)
	}
	menu := listKeyboards(t, h, c)["menu.greeting"][0]
	if !menu.Overridden || menu.Inactive || menu.Rows[0][0] != "button.pay" || !slices.Contains(menu.Hidden, "button.promo") {
		t.Fatalf("menu: %+v", menu)
	}
	// неупомянутые кнопки не пропали, скрытых в рядах нет
	flat := slices.Concat(menu.Rows...)
	if !slices.Contains(flat, "button.support") || slices.Contains(flat, "button.promo") {
		t.Fatalf("rows: %v", menu.Rows)
	}

	// скрытая кнопка в ряду хранится только в hidden
	body = `{"rows":[["button.info","button.pay"]],"hidden":["button.pay"]}`
	if w := send(h, http.MethodPut, "/api/keyboards/menu", c, body); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := st.layouts["menu"].Rows; len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("canonical rows: %v", got)
	}

	// раскладка по умолчанию не хранится
	def, _ := texts.LookupKeyboard("menu")
	raw, _ := json.Marshal(map[string]any{"rows": def.Default, "hidden": []string{}})
	if w := send(h, http.MethodPut, "/api/keyboards/menu", c, string(raw)); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if _, ok := st.layouts["menu"]; ok {
		t.Fatal("default layout stored")
	}

	// сброс
	send(h, http.MethodPut, "/api/keyboards/menu", c, body)
	if w := send(h, http.MethodDelete, "/api/keyboards/menu", c, ""); w.Code != http.StatusNoContent {
		t.Fatalf("%d", w.Code)
	}
	if _, ok := st.layouts["menu"]; ok {
		t.Fatal("not reset")
	}
}

func TestPutKeyboardInvalid(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	tests := map[string]struct{ id, body, msg string }{
		"three in a row":     {"menu", `{"rows":[["button.info","button.pay","button.promo"]],"hidden":[]}`, "в ряду не больше 2 кнопок"},
		"unknown button":     {"menu", `{"rows":[["button.nope"]],"hidden":[]}`, `в этой клавиатуре нет кнопки "button.nope"`},
		"hide Yes":           {"confirm", `{"rows":[["button.no"],["button.have_promo"]],"hidden":["button.yes"]}`, "нельзя"},
		"drop No":            {"confirm", `{"rows":[["button.yes"]],"hidden":[]}`, "нельзя"},
		"duplicate":          {"menu", `{"rows":[["button.info"],["button.info"]],"hidden":[]}`, "дважды"},
		"block with sibling": {"tariffs", `{"rows":[["@tariffs","button.back"]],"hidden":[]}`, "один"},
		"empty after hiding": {"menu", `{"rows":[["button.pay"]],"hidden":["button.pay"]}`, "пустым"},
	}
	for name, tc := range tests {
		w := send(h, http.MethodPut, "/api/keyboards/"+tc.id, c, tc.body)
		var out struct{ Error, Message string }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		if w.Code != http.StatusBadRequest || out.Error != "invalid" || !contains(out.Message, tc.msg) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if len(st.layouts) != 0 {
		t.Fatalf("invalid layouts saved: %v", st.layouts)
	}
	if w := send(h, http.MethodPut, "/api/keyboards/nope", c, `{"rows":[]}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown keyboard: %d", w.Code)
	}
	if w := send(h, http.MethodDelete, "/api/keyboards/nope", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown keyboard: %d", w.Code)
	}
	if w := send(h, http.MethodPut, "/api/keyboards/menu", c, `not json`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", w.Code)
	}
}

func TestKeyboardsAuth(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	for _, m := range []string{http.MethodPut, http.MethodDelete} {
		if w := send(h, m, "/api/keyboards/menu", "", `{"rows":[]}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without session: %d", m, w.Code)
		}
		if w := do(h, m, "/api/keyboards/menu", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s non-json: %d", m, w.Code)
		}
	}
}

// Сохранённая раскладка, не проходящая проверку, не применяется: в ответе раскладка по умолчанию и причина.
func TestTextsInactiveKeyboard(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.layouts = map[string]texts.Layout{"confirm": {Rows: [][]string{{"button.yes"}}, Hidden: []string{"button.no"}}}
	k := listKeyboards(t, h, c)["payment.confirm"][0]
	if !k.Overridden || !k.Inactive || k.Problem == "" || !slices.EqualFunc(k.Rows, k.Default, slices.Equal) {
		t.Fatalf("kb: %+v", k)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
