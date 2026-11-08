package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/texts"
)

type cellJSON struct {
	ID, Key, Label, Style, Action string
	Conditions                    []texts.CondRef
	Locked, Template              bool
	Effective                     *struct{ Label, Preview, Style string }
}

type textsJSON struct {
	Items []struct {
		Key      string
		Keyboard *struct {
			Rows, Default [][]cellJSON
			Overridden    bool
			Locked        []string
			Inactive      bool
			Problem       string
		}
	}
	Conditions []struct{ ID, Title, Description string }
	Actions    []struct {
		ID, Kind string
		Bound    bool
	}
	Templates []struct{ ID, Title string }
}

func getTexts(t *testing.T, h http.Handler, c string) textsJSON {
	t.Helper()
	w := send(h, http.MethodGet, "/api/texts", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var out textsJSON
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (o textsJSON) kb(key string) *struct {
	Rows, Default [][]cellJSON
	Overridden    bool
	Locked        []string
	Inactive      bool
	Problem       string
} {
	for _, it := range o.Items {
		if it.Key == key {
			return it.Keyboard
		}
	}
	return nil
}

func TestTextsMessageKeyboardShape(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.overrides = map[string]string{"button.pay": "Заплатить"}
	out := getTexts(t, h, c)
	if len(out.Conditions) != len(texts.Conditions()) || len(out.Actions) == 0 || len(out.Templates) != 3 {
		t.Fatalf("catalogs: %d %d %d", len(out.Conditions), len(out.Actions), len(out.Templates))
	}
	if out.kb("menu.status_active") != nil {
		t.Error("message without buttons must have keyboard null")
	}
	greet := out.kb("menu.greeting")
	if greet == nil || greet.Overridden || len(greet.Rows) != 7 || len(greet.Locked) != 1 || greet.Locked[0] != "buy" {
		t.Fatalf("greeting: %+v", greet)
	}
	if out.kb("payment.confirmed") == nil {
		t.Error("menu messages share the greeting keyboard")
	}
	var pay cellJSON
	for _, row := range greet.Rows {
		for _, c := range row {
			if c.ID == "pay" {
				pay = c
			}
		}
	}
	if pay.Label != "" || pay.Effective == nil || pay.Effective.Label != "Заплатить" || pay.Action != "route:pay" || len(pay.Conditions) != 1 || pay.Conditions[0].Not {
		t.Errorf("pay cell: %+v", pay)
	}
	tr := out.kb("payment.tariffs").Rows[0][0]
	if !tr.Template || tr.Action != "template:tariffs" || tr.Effective != nil || !tr.Locked {
		t.Errorf("template cell: %+v", tr)
	}
}

func TestPutMessageKeyboard(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	def, _, _ := texts.MessageDefault("referral.menu")
	custom := def.Clone()
	custom.Rows[1][0].Label = "Домой"
	custom.Rows = append(custom.Rows, []texts.Cell{{ID: "site", Label: "Сайт", Action: "url:https://example.com", Conditions: []texts.CondRef{{ID: "referral_on"}}}})
	raw, _ := json.Marshal(map[string]any{"rows": custom.Rows})
	if w := send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, string(raw)); w.Code != http.StatusNoContent {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	if got := st.msgKbs["referral.menu"]; !got.Equal(custom) {
		t.Fatalf("stored: %+v", got)
	}
	kb := getTexts(t, h, c).kb("referral.menu")
	if !kb.Overridden || kb.Inactive || len(kb.Rows) != 3 || kb.Rows[1][0].Label != "Домой" || len(kb.Default) != 2 {
		t.Fatalf("after put: %+v", kb)
	}
	// раскладка, равная умолчанию, не хранится
	raw, _ = json.Marshal(map[string]any{"rows": def.Rows})
	if w := send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, string(raw)); w.Code != http.StatusNoContent {
		t.Fatalf("put default: %d", w.Code)
	}
	if _, ok := st.msgKbs["referral.menu"]; ok {
		t.Error("default layout stored")
	}
	send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, string(mustJSON(custom.Rows)))
	if w := send(h, http.MethodDelete, "/api/texts/referral.menu/keyboard", c, ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if _, ok := st.msgKbs["referral.menu"]; ok {
		t.Error("not reset")
	}
}

func mustJSON(rows [][]texts.Cell) []byte {
	b, _ := json.Marshal(map[string]any{"rows": rows})
	return b
}

func TestPutMessageKeyboardInvalid(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	def, _, _ := texts.MessageDefault("referral.menu")
	noBack := def.Clone()
	noBack.Rows = noBack.Rows[:1]
	badURL := def.Clone()
	badURL.Rows = append(badURL.Rows, []texts.Cell{{ID: "x", Label: "x", Action: "url:http://example.com", Conditions: []texts.CondRef{}}})
	unknownCond := def.Clone()
	unknownCond.Rows[0][0].Conditions = []texts.CondRef{{ID: "nope"}}
	for name, rows := range map[string][][]texts.Cell{"no back": noBack.Rows, "http url": badURL.Rows, "unknown condition": unknownCond.Rows} {
		w := send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, string(mustJSON(rows)))
		var body struct{ Error, Message string }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != http.StatusBadRequest || body.Error != "invalid" || !strings.ContainsAny(body.Message, "абвгдежзиклмнопрстуфхцчшщыэюя") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if len(st.msgKbs) != 0 {
		t.Error("invalid layout stored")
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodPut, "/api/texts/nope/keyboard", http.StatusNotFound},
		{http.MethodPut, "/api/texts/menu.status_active/keyboard", http.StatusNotFound}, // простой текст: клавиатура не положена
		{http.MethodDelete, "/api/texts/nope/keyboard", http.StatusNotFound},
	} {
		if w := send(h, tc.method, tc.path, c, `{"rows":[]}`); w.Code != tc.want {
			t.Errorf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	if w := send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
	if w := send(h, http.MethodPut, "/api/texts/referral.menu/keyboard", c, `{}`); w.Code != http.StatusBadRequest {
		t.Errorf("no rows: %d", w.Code)
	}
}

func TestMessageKeyboardAuth(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	for _, m := range []string{http.MethodPut, http.MethodDelete} {
		if w := send(h, m, "/api/texts/referral.menu/keyboard", "", `{"rows":[]}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without session: %d", m, w.Code)
		}
		if w := do(h, m, "/api/texts/referral.menu/keyboard", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s non-json: %d", m, w.Code)
		}
	}
}

// Сохранённая клавиатура, не проходящая проверку, не применяется: в rows клавиатура по умолчанию и причина.
func TestTextsInactiveMessageKeyboard(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	def, _, _ := texts.MessageDefault("referral.menu")
	bad := def.Clone()
	bad.Rows = bad.Rows[:1]
	st.msgKbs = map[string]texts.MsgLayout{"referral.menu": bad}
	kb := getTexts(t, h, c).kb("referral.menu")
	if !kb.Overridden || !kb.Inactive || kb.Problem == "" || len(kb.Rows) != 2 {
		t.Fatalf("kb: %+v", kb)
	}
}
