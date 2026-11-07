package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const botKeys = `"reminderTimes":["18:00","09:30"],"advanceLimit":2,"advanceHours":48,"reminderWeekDays":5,"reminderDayHours":12,"devicesBase":4,"devicePrice":50,"deviceOptions":[1,4],"deviceMax":8,"miniAppUrl":"https://sub.example/app","trialDays":5,"trialDevices":3,"referralDays":9`

const goodSettings = `{"prices":{"1":200,"3":500},"phone":"+7 111","bank":"Сбер","comments":[" Кофе ","Чай"],"supportUrl":"https://t.me/new",` + botKeys + `}`

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
	if len(st.setRaw) != 19 {
		t.Fatalf("saved keys = %v", st.setRaw)
	}
	w := send(h, http.MethodGet, "/api/settings", c, "")
	var out struct {
		Prices     map[string]int
		Phone      string
		Bank       string
		Comments   []string
		SupportURL string

		ReminderTimes    []string
		AdvanceLimit     int
		AdvanceHours     int
		ReminderWeekDays int
		ReminderDayHours int
		DevicesBase      int
		DevicePrice      int
		TrialDays        int
		TrialDevices     int
		ReferralDays     int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.TrialDays != 5 || out.TrialDevices != 3 {
		t.Fatalf("trial: %s", w.Body)
	}
	if out.ReferralDays != 9 {
		t.Fatalf("referral: %s", w.Body)
	}
	if out.DevicesBase != 4 || out.DevicePrice != 50 {
		t.Fatalf("devices: %s", w.Body)
	}
	if strings.Join(out.ReminderTimes, ",") != "09:30,18:00" || out.AdvanceLimit != 2 || out.AdvanceHours != 48 || out.ReminderWeekDays != 5 || out.ReminderDayHours != 12 {
		t.Fatalf("bot keys: %s", w.Body)
	}
	if out.Prices["3"] != 500 || len(out.Prices) != 2 || out.Phone != "+7 111" || out.Bank != "Сбер" ||
		strings.Join(out.Comments, "|") != "Кофе|Чай" || out.SupportURL != "https://t.me/new" {
		t.Fatalf("got %s", w.Body)
	}
}

func TestSettingsVarsCRUD(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	put := func(vars string) *httptest.ResponseRecorder {
		body := strings.TrimSuffix(goodSettings, "}") + `,"vars":` + vars + `}`
		return send(h, http.MethodPut, "/api/settings", c, body)
	}
	if w := put(`{"promoLink":"https://x.io"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w := send(h, http.MethodGet, "/api/settings", c, "")
	if !strings.Contains(w.Body.String(), `"vars":{"promoLink":"https://x.io"}`) {
		t.Fatalf("get = %s", w.Body)
	}

	// имя системной и невалидное имя
	for _, vars := range []string{`{"name":"x"}`, `{"months":"x"}`, `{"1a":"x"}`, `{"a":""}`} {
		if w := put(vars); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", vars, w.Code, w.Body)
		}
	}

	// текст с переменной сохраняется; удалить её теперь нельзя, ключи названы
	if w := send(h, http.MethodPut, "/api/texts/payment.tariffs", c, `{"body":"Цена {{.promoLink}}"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = put(`{}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "promoLink") || !strings.Contains(w.Body.String(), "payment.tariffs") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(string(st.setRaw["vars"]), "promoLink") {
		t.Fatalf("vars = %s", st.setRaw["vars"])
	}
	// значение менять можно, текст убран — удалять тоже
	if w := put(`{"promoLink":"https://y.io"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := send(h, http.MethodDelete, "/api/texts/payment.tariffs", c, ""); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := put(`{}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// без поля vars — пустой объект, а не null: null Merge считает битым
	if w := send(h, http.MethodPut, "/api/settings", c, goodSettings); w.Code != http.StatusNoContent || string(st.setRaw["vars"]) != "{}" {
		t.Fatalf("%d %s %s", w.Code, w.Body, st.setRaw["vars"])
	}
}

func TestSettingsInvalid(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	w := send(h, http.MethodPut, "/api/settings", c, `{"prices":{},"phone":"+7","bank":"B","comments":["a"],"supportUrl":"https://x.io",`+botKeys+`}`)
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
	w = send(h, http.MethodPut, "/api/settings", c, `{"prices":{"2":0,"1":0},"phone":"","bank":"B","comments":["a"],"supportUrl":"x",`+botKeys+`}`)
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

// Значение своей переменной нельзя менять так, чтобы сохранённая кнопка перестала влезать в 64 символа.
func TestSettingsVarValueMustFitSavedButtons(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	put := func(value string) *httptest.ResponseRecorder {
		body := strings.TrimSuffix(goodSettings, "}") + `,"vars":{"promoLink":"` + value + `"}}`
		return send(h, http.MethodPut, "/api/settings", c, body)
	}
	if w := put("x"); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	st.setRaw["vars"] = []byte(`{"promoLink":"x"}`)
	// кнопка с переменной сохраняется: 4 + 1 символ
	if w := send(h, http.MethodPut, "/api/texts/button.pay", c, `{"body":"Для {{.promoLink}}"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	// 60 + 4 = 64: проходит
	if w := put(strings.Repeat("я", 60)); w.Code != http.StatusNoContent {
		t.Fatalf("64: %d %s", w.Code, w.Body)
	}
	st.setRaw["vars"] = []byte(`{"promoLink":"x"}`)
	// 61 + 4 = 65: отказ с ключом
	w := put(strings.Repeat("я", 61))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "promoLink") || !strings.Contains(w.Body.String(), "button.pay") {
		t.Fatalf("65: %d %s", w.Code, w.Body)
	}
	// переменная без изменений не проверяется заново; значение в сообщении (не кнопка) может быть длинным
	if w := put("x"); w.Code != http.StatusNoContent {
		t.Fatalf("unchanged: %d %s", w.Code, w.Body)
	}
}

// Значение своей переменной: 96 сохраняется, 97 — 400 с русским сообщением; уже сохранённое длиннее не роняет бота на
// дефолты, панель показывает problems, а сохранить настройки, не сократив, нельзя.
func TestSettingsVarLengthLimit(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	put := func(v string) *httptest.ResponseRecorder {
		body := strings.TrimSuffix(goodSettings, "}") + `,"vars":{"my":"` + v + `"}}`
		return send(h, http.MethodPut, "/api/settings", c, body)
	}
	if w := put(strings.Repeat("a", 96)); w.Code != http.StatusNoContent {
		t.Fatalf("96: %d %s", w.Code, w.Body)
	}
	w := put(strings.Repeat("a", 97))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Переменная my длиннее 96 символов: сократите") {
		t.Fatalf("97: %d %s", w.Code, w.Body)
	}

	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"` + strings.Repeat("a", 200) + `"}`), "phone": []byte(`"+7 222"`)}
	var out struct {
		UsingDefaults bool
		Problems      []string
		Phone         string
	}
	w = send(h, http.MethodGet, "/api/settings", c, "")
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.UsingDefaults || out.Phone != "+7 222" ||
		len(out.Problems) != 1 || out.Problems[0] != "Переменная my длиннее 96 символов: сократите" {
		t.Fatalf("legacy: %s", w.Body)
	}
}
