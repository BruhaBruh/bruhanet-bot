package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const goodSettings = `{"prices":{"1":200,"3":500},"phone":"+7 111","bank":"Сбер","comments":[" Кофе ","Чай"],"supportUrl":"https://t.me/new"}`

func TestSettingsGetDefaults(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	w := send(h, http.MethodGet, "/api/settings", c, "")
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if out["phone"] != "+7 900" || out["bank"] != "Т-Банк" || out["supportUrl"] != "https://t.me/support" ||
		out["prices"].(map[string]any)["6"] != float64(800) || len(out["comments"].([]any)) != 4 {
		t.Fatalf("defaults = %s", w.Body)
	}
}

func TestSettingsPutThenGet(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	if w := send(h, http.MethodPut, "/api/settings", c, goodSettings); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(st.setRaw) != 5 {
		t.Fatalf("saved keys = %v", st.setRaw)
	}
	w := send(h, http.MethodGet, "/api/settings", c, "")
	var out struct {
		Prices     map[string]int
		Phone      string
		Bank       string
		Comments   []string
		SupportURL string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Prices["3"] != 500 || len(out.Prices) != 2 || out.Phone != "+7 111" || out.Bank != "Сбер" ||
		strings.Join(out.Comments, "|") != "Кофе|Чай" || out.SupportURL != "https://t.me/new" {
		t.Fatalf("got %s", w.Body)
	}
}

func TestSettingsInvalid(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	w := send(h, http.MethodPut, "/api/settings", c, `{"prices":{},"phone":"+7","bank":"B","comments":["a"],"supportUrl":"https://x.io"}`)
	var out struct {
		Error    string
		Message  string
		Problems []string
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusBadRequest || out.Error != "invalid" || out.Message != "нужен хотя бы один тариф" || len(out.Problems) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	// несколько проблем — через «; », без служебного префикса
	w = send(h, http.MethodPut, "/api/settings", c, `{"prices":{"2":0,"1":0},"phone":"","bank":"B","comments":["a"],"supportUrl":"x"}`)
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusBadRequest || strings.Contains(out.Message, "некорректная настройка") ||
		!strings.Contains(out.Message, "на 1 мес.") || strings.Index(out.Message, "на 1 мес.") > strings.Index(out.Message, "на 2 мес.") ||
		len(out.Problems) != 4 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	for _, body := range []string{`{`, `{"prices":"x"}`} {
		if w := send(h, http.MethodPut, "/api/settings", c, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	if len(st.setRaw) != 0 {
		t.Fatalf("invalid saved: %v", st.setRaw)
	}
}

func TestSettingsGetInvalidStoredUsesDefaults(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.setRaw = map[string][]byte{"prices": []byte(`{"1":0}`)}
	w := send(h, http.MethodGet, "/api/settings", c, "")
	var out struct {
		UsingDefaults bool
		Problems      []string
		Phone         string
		Prices        map[string]int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !out.UsingDefaults || len(out.Problems) != 1 || out.Phone != "+7 900" || out.Prices["6"] != 800 {
		t.Fatalf("got %s", w.Body)
	}
	// валидные — без флага
	st.setRaw = nil
	if w := send(h, http.MethodGet, "/api/settings", c, ""); strings.Contains(w.Body.String(), "usingDefaults") {
		t.Fatalf("flag on valid: %s", w.Body)
	}
}
