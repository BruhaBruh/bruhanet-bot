package server_test

import (
	"encoding/json"
	"fmt"
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

type bcResp struct {
	Broadcast model.Broadcast `json:"broadcast"`
}

// Review Focus 1: двойной клик «Разослать» с тем же ключом — одна рассылка.
func TestBroadcastCreateIdempotent(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.bcRecipients = []int64{5, 7}
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
	h, st, c := bcServer(t, &fakePanel{})
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
	h, st, c := bcServer(t, &fakePanel{})
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
	h, st, c := bcServer(t, &fakePanel{})
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
		st.lastBF.Limit != 20 || st.lastBF.Offset != 20 {
		t.Fatalf("out = %+v, f = %+v", out, st.lastBF)
	}
	if w := send(h, http.MethodGet, "/api/broadcasts?page=0", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("page=0: %d", w.Code)
	}
}

// Худший случай после подстановки: ошибка называет переменную; граница, эмодзи = 2 единицы UTF-16, личные — те же правила.
func TestMessageWorstCaseNamesVar(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.setRaw = map[string][]byte{"vars": []byte(`{"my":"a"}`)}
	post := func(path string, in map[string]any) (int, string) {
		w := send(h, http.MethodPost, path, c, jsonBody(t, in))
		return w.Code, w.Body.String()
	}
	mk := func(text, photo string) map[string]any {
		return map[string]any{"idempotencyKey": uuid.New(), "text": text, "photo": photo}
	}
	// my считается по максимуму (96): 928 + 96 = 1024 в подпись, на единицу больше — 400 с именем
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 928)+"{{.my}}", "AgAD")); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 929)+"{{.my}}", "AgAD")); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
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
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 4000)+"{{.my}}", "")); code != http.StatusCreated {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/broadcasts", mk(strings.Repeat("a", 4001)+"{{.my}}", "")); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := post("/api/users/7/messages", map[string]any{"text": strings.Repeat("a", 4001) + "{{.my}}"}); code != http.StatusBadRequest || !strings.Contains(body, "{{.my}}") {
		t.Fatalf("direct: %d %s", code, body)
	}
}

func postBC(t *testing.T, h http.Handler, c string, in map[string]any) (int, model.Broadcast) {
	t.Helper()
	w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, in))
	var out bcResp
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Broadcast
}

// now = 2026-11-02 12:00 UTC = 15:00 МСК. Время приходит по Москве, хранится в UTC; прошедшее и кривое — 400.
func TestBroadcastSchedule(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.bcRecipients = []int64{5}
	key := uuid.New()
	in := map[string]any{"idempotencyKey": key, "text": "позже", "scheduledAt": "2026-11-02T18:30"}
	code, b := postBC(t, h, c, in)
	want := time.Date(2026, 11, 2, 15, 30, 0, 0, time.UTC)
	if code != http.StatusCreated || b.Status != "scheduled" || b.ScheduledAt == nil || !b.ScheduledAt.Equal(want) || b.Total != 0 {
		t.Fatalf("%d %+v", code, b)
	}
	if len(st.broadcasts[0].recipients) != 0 {
		t.Fatal("scheduled must not resolve recipients")
	}
	// двойной клик: одна строка
	if code, again := postBC(t, h, c, in); code != http.StatusOK || again.ID != b.ID || len(st.broadcasts) != 1 {
		t.Fatalf("repeat: %d %+v n=%d", code, again, len(st.broadcasts))
	}
	for at, wantCode := range map[string]int{
		"2026-11-02T15:01": http.StatusCreated,
		"2026-11-02T15:00": http.StatusBadRequest, // ровно сейчас — уже не будущее
		"2026-11-02T14:59": http.StatusBadRequest,
		"2026-11-01T10:00": http.StatusBadRequest,
		// пояс и секунды не принимаем: поле всегда московское
		"2026-11-02T18:30:00Z":      http.StatusBadRequest,
		"2026-11-02T18:30:00+03:00": http.StatusBadRequest,
		"2026-11-02 18:30":          http.StatusBadRequest,
		"2026-13-02T18:30":          http.StatusBadRequest,
		"завтра":                    http.StatusBadRequest,
	} {
		code, _ := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x", "scheduledAt": at})
		if code != wantCode {
			t.Errorf("%q: %d, want %d", at, code, wantCode)
		}
	}
	// без времени — сразу
	if code, now := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x"}); code != http.StatusCreated || now.Status != "sending" || now.Total != 1 {
		t.Fatalf("now: %d %+v", code, now)
	}
}

func TestBroadcastEditCancelGet(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.bcRecipients = []int64{5}
	_, sched := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "v1", "scheduledAt": "2026-11-03T10:00"})
	_, sent := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "now"})
	put := func(id any, body map[string]any) (int, string) {
		w := send(h, http.MethodPut, "/api/broadcasts/"+fmt.Sprint(id), c, jsonBody(t, body))
		return w.Code, w.Body.String()
	}

	code, body := put(sched.ID, map[string]any{"text": "v2", "scheduledAt": "2026-11-04T09:15"})
	var out bcResp
	_ = json.Unmarshal([]byte(body), &out)
	if code != http.StatusOK || out.Broadcast.Text != "v2" || !out.Broadcast.ScheduledAt.Equal(time.Date(2026, 11, 4, 6, 15, 0, 0, time.UTC)) {
		t.Fatalf("edit: %d %s", code, body)
	}
	for name, tc := range map[string]struct {
		id   any
		body map[string]any
		want int
	}{
		"no time":       {sched.ID, map[string]any{"text": "x"}, http.StatusBadRequest},
		"past":          {sched.ID, map[string]any{"text": "x", "scheduledAt": "2026-11-02T10:00"}, http.StatusBadRequest},
		"empty text":    {sched.ID, map[string]any{"text": "", "scheduledAt": "2026-11-04T09:15"}, http.StatusBadRequest},
		"already sent":  {sent.ID, map[string]any{"text": "x", "scheduledAt": "2026-11-04T09:15"}, http.StatusConflict},
		"unknown":       {uuid.New(), map[string]any{"text": "x", "scheduledAt": "2026-11-04T09:15"}, http.StatusNotFound},
		"bad id":        {"nope", map[string]any{"text": "x", "scheduledAt": "2026-11-04T09:15"}, http.StatusBadRequest},
		"unclosed html": {sched.ID, map[string]any{"text": "<b>x", "scheduledAt": "2026-11-04T09:15"}, http.StatusBadRequest},
	} {
		if code, body := put(tc.id, tc.body); code != tc.want {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	// содержимое не испортилось отказами
	if w := send(h, http.MethodGet, "/api/broadcasts/"+sched.ID.String(), c, ""); !strings.Contains(w.Body.String(), `"text":"v2"`) {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}

	cancel := func(id any) int {
		return send(h, http.MethodPost, "/api/broadcasts/"+fmt.Sprint(id)+"/cancel", c, `{}`).Code
	}
	if cancel(sent.ID) != http.StatusConflict || cancel(uuid.New()) != http.StatusNotFound || cancel("nope") != http.StatusBadRequest {
		t.Fatal("cancel errors")
	}
	first := cancel(sched.ID)
	if first != http.StatusOK || cancel(sched.ID) != http.StatusOK {
		t.Fatal("cancel scheduled (repeat is harmless)")
	}
	if code, _ := put(sched.ID, map[string]any{"text": "x", "scheduledAt": "2026-11-04T09:15"}); code != http.StatusConflict {
		t.Fatalf("edit cancelled: %d", code)
	}
	if w := send(h, http.MethodGet, "/api/broadcasts/"+uuid.New().String(), c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("get unknown: %d", w.Code)
	}
}

func TestBroadcastListStatusFilter(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "a"})
	postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "b", "scheduledAt": "2026-11-03T10:00"})
	list := func(q string) (int, int, []string) {
		w := send(h, http.MethodGet, "/api/broadcasts"+q, c, "")
		var out struct {
			Items []model.Broadcast `json:"items"`
			Total int               `json:"total"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		texts := make([]string, 0, len(out.Items))
		for _, b := range out.Items {
			texts = append(texts, b.Text)
		}
		return w.Code, out.Total, texts
	}
	if code, total, got := list("?status=scheduled"); code != 200 || total != 1 || !slices.Equal(got, []string{"b"}) {
		t.Fatalf("scheduled: %d %d %v", code, total, got)
	}
	if code, total, _ := list("?status=sending,sent"); code != 200 || total != 1 || !slices.Equal(st.lastBF.Statuses, []string{"sending", "sent"}) {
		t.Fatalf("sent: %d %d %v", code, total, st.lastBF.Statuses)
	}
	if code, total, _ := list(""); code != 200 || total != 2 {
		t.Fatalf("all: %d %d", code, total)
	}
	if code, _, _ := list("?status=bogus"); code != http.StatusBadRequest {
		t.Fatalf("bogus: %d", code)
	}
}

func TestBroadcastRecipients(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	st.counts = model.AudienceCounts{All: 10, Active: 6, NoSubscription: 4, Expiring: 2}
	get := func(q string) (int, string) {
		w := send(h, http.MethodGet, "/api/broadcasts/recipients"+q, c, "")
		return w.Code, strings.TrimSpace(w.Body.String())
	}
	if code, body := get(""); code != 200 || body != `{"active":6,"all":10,"no_subscription":4}` {
		t.Fatalf("all: %d %s", code, body)
	}
	if code, body := get("?days=7"); code != 200 || body != `{"active":6,"all":10,"expiring":2,"no_subscription":4}` || st.lastDays != 7 {
		t.Fatalf("with days: %d %s days=%d", code, body, st.lastDays)
	}
	for q, want := range map[string]string{
		"?audience=all":              `{"count":10}`,
		"?audience=active":           `{"count":6}`,
		"?audience=no_subscription":  `{"count":4}`,
		"?audience=expiring&days=3":  `{"count":2}`,
		"?audience=active&days=3":    `{"count":6}`, // дни нужны только expiring
		"?audience=expiring&days=60": `{"count":2}`,
	} {
		if code, body := get(q); code != 200 || body != want {
			t.Errorf("%s: %d %s", q, code, body)
		}
	}
	for _, q := range []string{"?audience=trial", "?audience=expiring", "?audience=expiring&days=0", "?audience=expiring&days=61", "?days=x", "?days=-1&audience=expiring"} {
		if code, body := get(q); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", q, code, body)
		}
	}
	// days вне границ без audience — просто без expiring, а не 400
	if code, body := get("?days=999"); code != 200 || strings.Contains(body, "expiring") {
		t.Errorf("days out of range: %d %s", code, body)
	}
}

func TestBroadcastAudienceInBody(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	three := 3
	code, b := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x", "audience": "expiring", "audienceDays": three})
	if code != http.StatusCreated || b.Audience != "expiring" || b.AudienceDays == nil || *b.AudienceDays != 3 {
		t.Fatalf("%d %+v", code, b)
	}
	// по умолчанию — все
	if code, b = postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x"}); code != http.StatusCreated || b.Audience != "all" || b.AudienceDays != nil {
		t.Fatalf("default: %d %+v", code, b)
	}
	n := len(st.broadcasts)
	for name, in := range map[string]map[string]any{
		"unknown":       {"audience": "trial"},
		"no days":       {"audience": "expiring"},
		"days too many": {"audience": "expiring", "audienceDays": 61},
		"days zero":     {"audience": "expiring", "audienceDays": 0},
		"days on all":   {"audience": "all", "audienceDays": 3},
	} {
		in["idempotencyKey"], in["text"] = uuid.New(), "x"
		if code, _ := postBC(t, h, c, in); code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if len(st.broadcasts) != n {
		t.Fatal("refused must not create")
	}
	// правка запланированной тоже меняет аудиторию
	_, sched := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x", "scheduledAt": "2026-11-03T10:00"})
	w := send(h, http.MethodPut, "/api/broadcasts/"+sched.ID.String(), c, jsonBody(t, map[string]any{"text": "x", "scheduledAt": "2026-11-03T10:00", "audience": "active"}))
	if !strings.Contains(w.Body.String(), `"audience":"active"`) {
		t.Fatalf("edit: %d %s", w.Code, w.Body)
	}
}

func TestBroadcastButtonsAndSilent(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	btn := func(text, url, style string) map[string]any {
		return map[string]any{"text": text, "url": url, "style": style}
	}
	code, b := postBC(t, h, c, map[string]any{
		"idempotencyKey": uuid.New(), "text": "x", "silent": true,
		"buttons": []any{btn("Сайт", "https://e.com/?a=1", "primary"), btn("Ещё", "https://e.org", "")},
	})
	if code != http.StatusCreated || !b.Silent || len(b.Buttons) != 2 || b.Buttons[0].URL != "https://e.com/?a=1" || b.Buttons[0].Style != "primary" {
		t.Fatalf("%d %+v", code, b)
	}
	n := len(st.broadcasts)
	long := strings.Repeat("я", 65)
	for name, buttons := range map[string][]any{
		"javascript": {btn("x", "javascript:alert(1)", "")},
		"data":       {btn("x", "data:text/html;base64,PHNjcmlwdD4=", "")},
		"http":       {btn("x", "http://e.com", "")},
		"tg":         {btn("x", "tg://user?id=1", "")},
		"vars":       {btn("{{.name}}", "https://e.com", "")},
		"long text":  {btn(long, "https://e.com", "")},
		"empty":      {btn("", "https://e.com", "")},
		"style":      {btn("x", "https://e.com", "rainbow")},
		"four":       {btn("1", "https://e.com", ""), btn("2", "https://e.com", ""), btn("3", "https://e.com", ""), btn("4", "https://e.com", "")},
	} {
		code, _ := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x", "buttons": buttons})
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, code)
		}
	}
	if len(st.broadcasts) != n {
		t.Fatal("refused must not create")
	}
	// без кнопок — пустой массив в ответе, не null
	if _, b := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x"}); b.Buttons == nil || b.Silent {
		t.Fatalf("defaults: %+v", b)
	}
	// правка запланированной принимает и проверяет кнопки
	_, sched := postBC(t, h, c, map[string]any{"idempotencyKey": uuid.New(), "text": "x", "scheduledAt": "2026-11-03T10:00"})
	put := func(buttons []any) int {
		return send(h, http.MethodPut, "/api/broadcasts/"+sched.ID.String(), c,
			jsonBody(t, map[string]any{"text": "x", "scheduledAt": "2026-11-03T10:00", "silent": true, "buttons": buttons})).Code
	}
	if put([]any{btn("a", "https://e.com", "")}) != http.StatusOK || put([]any{btn("a", "javascript:1", "")}) != http.StatusBadRequest {
		t.Fatal("edit buttons")
	}
}

// «Отправить себе» шлёт админу сообщение с теми же кнопками и «без звука».
func TestBroadcastPreviewSendButtons(t *testing.T) {
	h, st, c := bcServer(t, &fakePanel{})
	body := jsonBody(t, map[string]any{"text": "x", "silent": true, "buttons": []any{map[string]any{"text": "a", "url": "https://e.com", "style": "success"}}})
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, body); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	m := st.outbox[len(st.outbox)-1]
	if !m.Silent || m.Keyboard == nil || m.Keyboard.InlineKeyboard[0][0].URL != "https://e.com" || m.Keyboard.InlineKeyboard[0][0].Style != "success" {
		t.Fatalf("outbox = %+v", m)
	}
	bad := jsonBody(t, map[string]any{"text": "x", "buttons": []any{map[string]any{"text": "a", "url": "javascript:1"}}})
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, bad); w.Code != http.StatusBadRequest {
		t.Fatalf("bad: %d", w.Code)
	}
}
