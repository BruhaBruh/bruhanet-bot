package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

func bcServer(t *testing.T, p *fakePanel) (http.Handler, *fakeStore, string) {
	t.Helper()
	h, st := newServerPanel(t, "http://localhost:5173", p)
	return h, st, st.addSession(t, adminID, now.Add(time.Hour))
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Переменные в рассылках: неизвестная и неразборчивая — понятный 400, подпись — по худшему случаю.
func TestMessageVars(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"a"}`)}
	post := func(text, photo string) (int, string) {
		w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, jsonBody(t, map[string]string{"text": text, "photo": photo}))
		return w.Code, w.Body.String()
	}
	if code, body := post("Привет, {{.name}} {{.my}}", ""); code != http.StatusNoContent {
		t.Fatalf("%d %s", code, body)
	}
	last := st.outbox[len(st.outbox)-1]
	if !last.Template || last.Text != "Привет, {{.name}} {{.my}}" || last.ChatID != adminID {
		t.Fatalf("outbox = %+v", last)
	}
	if code, body := post("обычный {{ текст", ""); code != http.StatusNoContent || st.outbox[len(st.outbox)-1].Template {
		t.Fatalf("%d %s %+v", code, body, st.outbox[len(st.outbox)-1])
	}
	for text, want := range map[string]string{
		"{{.nope}}":     "неизвестная переменная",
		"{{name}}":      "неправильная переменная",
		"{{.na me}}":    "неправильная переменная",
		"{{.months}} x": "неизвестная переменная", // переменная чужого текста в рассылке не имеет значения
	} {
		if code, body := post(text, ""); code != http.StatusBadRequest || !strings.Contains(body, want) {
			t.Errorf("%q: %d %s", text, code, body)
		}
	}
	// name 64: 960 + 64 = 1024 — помещается в подпись, на символ больше — нет
	if code, body := post(strings.Repeat("a", 960)+"{{.name}}", "AgAD"); code != http.StatusNoContent {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post(strings.Repeat("a", 961)+"{{.name}}", "AgAD"); code != http.StatusBadRequest || !strings.Contains(body, "подпись") {
		t.Fatalf("%d %s", code, body)
	}
	// без фото — 4096
	if code, body := post(strings.Repeat("a", 4033)+"{{.name}}", ""); code != http.StatusBadRequest {
		t.Fatalf("%d %s", code, body)
	}
	// личное сообщение — те же правила
	if w := send(h, http.MethodPost, "/api/users/7/messages", c, `{"text":"{{.nope}}"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestBroadcastPreviewSend(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})

	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, `{"text":"<b>Привет</b>","photo":"AgAD"}`); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	want := model.OutboxMessage{EventType: "text_test", ChatID: adminID, Text: "<b>Привет</b>", ParseMode: "HTML", Photo: "AgAD"}
	if len(st.outbox) != 1 || st.outbox[0] != want {
		t.Fatalf("outbox = %+v", st.outbox)
	}
	// фото без текста — можно
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, `{"text":"","photo":"AgAD"}`); w.Code != http.StatusNoContent {
		t.Fatalf("photo only: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"text":""}`, `{"text":"<b>x"}`, `{"text":"a & b"}`, `not json`} {
		if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if len(st.outbox) != 2 {
		t.Fatalf("outbox = %+v", st.outbox)
	}
}

func TestBroadcastRecipients(t *testing.T) {
	h, _, c := bcServer(t, &fakePanel{ids: []int{5, 0, 7, 5}})
	if w := send(h, http.MethodGet, "/api/broadcasts/recipients", c, ""); w.Code != http.StatusOK ||
		strings.TrimSpace(w.Body.String()) != `{"count":2}` {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	h, _, c = bcServer(t, &fakePanel{ids: []int{5}, usersErr: errors.New("boom")})
	if w := send(h, http.MethodGet, "/api/broadcasts/recipients", c, ""); w.Code != http.StatusBadGateway ||
		strings.TrimSpace(w.Body.String()) != `{"error":"panel_unavailable"}` {
		t.Fatalf("panel down: %d %s", w.Code, w.Body)
	}
}

func TestBroadcastRecipientsTimeout(t *testing.T) {
	h, st := newServerTimeout(t, "http://localhost:5173", blockingPanel{}, 20*time.Millisecond)
	c := st.addSession(t, adminID, now.Add(time.Hour))
	if w := send(h, http.MethodGet, "/api/broadcasts/recipients", c, ""); w.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

type bcResp struct {
	Broadcast model.Broadcast `json:"broadcast"`
}

// Review Focus 1: двойной клик «Разослать» с тем же ключом — одна рассылка.
func TestBroadcastCreateIdempotent(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{ids: []int{5, 7, 5}})
	key := uuid.New()
	body := jsonBody(t, map[string]any{"idempotencyKey": key, "text": "Новости", "photo": "AgAD"})

	w := send(h, http.MethodPost, "/api/broadcasts", c, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", w.Code, w.Body)
	}
	var first bcResp
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Broadcast.Text != "Новости" || first.Broadcast.Photo != "AgAD" || first.Broadcast.Total != 2 {
		t.Fatalf("broadcast = %+v", first.Broadcast)
	}

	w = send(h, http.MethodPost, "/api/broadcasts", c, body)
	if w.Code != http.StatusOK {
		t.Fatalf("repeat: %d %s", w.Code, w.Body)
	}
	var second bcResp
	if err := json.Unmarshal(w.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.Broadcast.ID != first.Broadcast.ID || len(st.broadcasts) != 1 {
		t.Fatalf("second = %+v, stored = %d", second.Broadcast, len(st.broadcasts))
	}
	b := st.broadcasts[0]
	if b.key != key || b.adminID != adminID || !slices.Equal(b.recipients, []int64{5, 7}) {
		t.Fatalf("stored = %+v", b)
	}
}

// Review Focus 3: подпись к фото длиннее 1024 — 400 с понятным текстом, ничего не создано.
func TestBroadcastCaptionLimit(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{ids: []int{5}})
	long := "<b>" + strings.Repeat("я", 1025) + "</b>"

	w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, map[string]any{"idempotencyKey": uuid.New(), "text": long, "photo": "AgAD"}))
	var e struct{ Error, Message string }
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || w.Code != http.StatusBadRequest ||
		e.Error != "invalid" || !strings.Contains(e.Message, "1024") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(st.broadcasts) != 0 {
		t.Fatal("must not create")
	}
	// ровно 1024 с фото и 1025 без фото — можно
	ok := []map[string]any{
		{"idempotencyKey": uuid.New(), "text": strings.Repeat("я", 1024), "photo": "AgAD"},
		{"idempotencyKey": uuid.New(), "text": long},
	}
	for _, in := range ok {
		if w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, in)); w.Code != http.StatusCreated {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, jsonBody(t, map[string]any{"text": long, "photo": "AgAD"})); w.Code != http.StatusBadRequest {
		t.Fatalf("preview: %d", w.Code)
	}
}

func TestBroadcastCreateErrors(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{ids: []int{5}, usersErr: errors.New("boom")})
	if w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, map[string]any{"idempotencyKey": uuid.New(), "text": "x"})); w.Code != http.StatusBadGateway ||
		strings.TrimSpace(w.Body.String()) != `{"error":"panel_unavailable"}` {
		t.Fatalf("panel down: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"text":"x"}`, `{"idempotencyKey":"nope","text":"x"}`, jsonBody(t, map[string]any{"idempotencyKey": uuid.New(), "text": ""})} {
		if w := send(h, http.MethodPost, "/api/broadcasts", c, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if len(st.broadcasts) != 0 {
		t.Fatal("must not create")
	}
}

func TestBroadcastList(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{ids: []int{5}})
	for range 21 {
		if w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, map[string]any{"idempotencyKey": uuid.New(), "text": "x"})); w.Code != http.StatusCreated {
			t.Fatal(w.Code)
		}
	}
	w := send(h, http.MethodGet, "/api/broadcasts?page=2", c, "")
	var out struct {
		Items []model.Broadcast `json:"items"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if out.Total != 21 || len(out.Items) != 1 || out.Items[0].ID != st.broadcasts[0].b.ID ||
		st.lastF.Limit != 20 || st.lastF.Offset != 20 {
		t.Fatalf("out = %+v, f = %+v", out, st.lastF)
	}
	if w := send(h, http.MethodGet, "/api/broadcasts?page=0", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("page=0: %d", w.Code)
	}
}

// Худший случай после подстановки: ошибка называет переменную; граница, эмодзи = 2 единицы UTF-16, личные — те же правила.
func TestMessageWorstCaseNamesVar(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{ids: []int{5}})
	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"a"}`)}
	post := func(path string, in map[string]any) (int, string) {
		w := send(h, http.MethodPost, path, c, jsonBody(t, in))
		return w.Code, w.Body.String()
	}
	mk := func(text, photo string) map[string]any {
		return map[string]any{"idempotencyKey": uuid.New(), "text": text, "photo": photo}
	}
	// my считается по максимуму (256): 768 + 256 = 1024 в подпись, на единицу больше — 400 с именем
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 768)+"{{.my}}", "AgAD")); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 769)+"{{.my}}", "AgAD")); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
		t.Fatalf("%d %s", code, body)
	}
	// эмодзи — 2 единицы: 512 помещаются, 513 нет
	if code, body := post("/api/broadcasts", mk(strings.Repeat("😀", 512), "AgAD")); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/broadcasts", mk(strings.Repeat("😀", 513), "AgAD")); code != http.StatusBadRequest {
		t.Fatalf("%d %s", code, body)
	}
	// без фото — 4096
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 3840)+"{{.my}}", "")); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 3841)+"{{.my}}", "")); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/users/7/messages", map[string]any{"text": strings.Repeat("a", 3841) + "{{.my}}"}); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
		t.Fatalf("direct: %d %s", code, body)
	}
}
