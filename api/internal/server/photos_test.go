package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func multipartBody(t *testing.T, field, name string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, name)
	if err == nil {
		_, err = fw.Write(data)
	}
	if err == nil {
		err = mw.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func upload(h http.Handler, cookie, contentType string, body *bytes.Buffer) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/photos", body)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
	}
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func photoServerSt(t *testing.T) (http.Handler, *fakeTG, *fakeStore, string) {
	t.Helper()
	h, st, tg := newServerTG(t, "http://localhost:5173", &fakePanel{}, 0)
	return h, tg, st, st.addSession(t, adminID, now.Add(time.Hour))
}

func TestPhotoUploadStoresBytesAndSkipsTelegram(t *testing.T) {
	h, tg, st, c := photoServerSt(t)
	img := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{1}, 1000)...)
	body, ct := multipartBody(t, "file", "a.png", img)

	w := upload(h, c, ct, body)
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK || len(out) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	id, err := uuid.Parse(out["photoId"])
	if err != nil {
		t.Fatalf("photoId %q: %v", out["photoId"], err)
	}
	if p := st.photos[id]; !bytes.Equal(p.content, img) || p.ct != "image/png" {
		t.Fatalf("stored = %d bytes %q", len(p.content), p.ct)
	}
	// api не умеет слать фото в Telegram (в интерфейсе нет SendPhoto), и файлов он не открывал
	if tg.opened != "" {
		t.Fatalf("telegram touched: %q", tg.opened)
	}
}

// Не картинка или больше 10 МБ — 400, в БД ничего не попадает. Тип берётся по содержимому, а не по имени файла.
func TestPhotoUploadRejects(t *testing.T) {
	h, _, st, c := photoServerSt(t)
	big := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{1}, 10<<20)...)

	for _, tc := range []struct {
		name, field, file string
		data              []byte
		msg               string
	}{
		{"text", "file", "x", []byte("hello, i am not an image"), "JPEG, PNG или WebP"},
		{"text named png", "file", "fake.png", []byte("<script>alert(1)</script>"), "JPEG, PNG или WebP"},
		{"gif", "file", "x", []byte("GIF89a......"), "JPEG, PNG или WebP"},
		{"big", "file", "x", big, "файл больше 10 МБ"},
		{"no file", "other", "x", pngHeader, "нет файла"},
	} {
		body, ct := multipartBody(t, tc.field, tc.file, tc.data)
		w := upload(h, c, ct, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.msg) {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body)
		}
	}
	if len(st.photos) != 0 {
		t.Fatalf("stored %d photos", len(st.photos))
	}
}

func TestPhotoUploadAuthAndType(t *testing.T) {
	h, _, _, c := photoServerSt(t)
	body, ct := multipartBody(t, "file", "a.png", pngHeader)
	if w := upload(h, "", ct, body); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := upload(h, c, "application/json", bytes.NewBufferString(`{}`)); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("json: %d", w.Code)
	}
	if w := upload(h, c, "", bytes.NewBufferString(`x`)); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("no content type: %d", w.Code)
	}
	// multipart принимает только этот роут — остальные мутирующие по-прежнему только JSON
	if w := upload(h, c, ct, body); w.Code == http.StatusUnsupportedMediaType {
		t.Fatal("photos must accept multipart")
	}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/broadcasts", strings.NewReader("x"))
	r.AddCookie(&http.Cookie{Name: "bn_session", Value: c})
	r.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("multipart to broadcasts: %d", w.Code)
	}
}

func TestPhotoGet(t *testing.T) {
	h, _, st, c := photoServerSt(t)
	// байты не PNG-заголовка достаточно: тип хранится при загрузке
	id, _ := st.SavePhoto(context.Background(), pngHeader, "image/png")

	w := do(h, http.MethodGet, "/api/photos/"+id.String(), c, "")
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), pngHeader) || w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	if w := do(h, http.MethodGet, "/api/photos/"+id.String(), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	for _, target := range []string{"/api/photos/" + uuid.NewString(), "/api/photos/nope"} {
		if w := do(h, http.MethodGet, target, c, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", target, w.Code)
		}
	}
}

// photoId вместо file_id: рассылка, черновик, личное и «Отправить мне» принимают загруженную картинку, чужой id и
// оба поля сразу отклоняются.
func TestPhotoIDInMessages(t *testing.T) {
	h, _, st, c := photoServerSt(t)
	pid, _ := st.SavePhoto(context.Background(), pngHeader, "image/png")

	// preview-send кладёт photo_id в outbox
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, jsonBody(t, map[string]any{"text": "hi", "photoId": pid})); w.Code != http.StatusNoContent {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	if len(st.outbox) != 1 || st.outbox[0].PhotoID != pid.String() || st.outbox[0].Photo != "" {
		t.Fatalf("outbox = %+v", st.outbox)
	}
	// рассылка
	key := uuid.New()
	w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, map[string]any{"idempotencyKey": key, "text": "Новости", "photoId": pid}))
	var b bcResp
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || w.Code != http.StatusCreated || b.Broadcast.PhotoID == nil || *b.Broadcast.PhotoID != pid {
		t.Fatalf("broadcast: %d %s", w.Code, w.Body)
	}
	// черновик (автосохранение) несёт photoId
	did := uuid.New()
	w = send(h, http.MethodPut, "/api/broadcasts/"+did.String(), c, jsonBody(t, map[string]any{"text": "", "photoId": pid}))
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || w.Code != http.StatusOK || b.Broadcast.PhotoID == nil || *b.Broadcast.PhotoID != pid {
		t.Fatalf("draft: %d %s", w.Code, w.Body)
	}
	// чужой/удалённый id и оба поля сразу
	other := uuid.New()
	for name, body := range map[string]map[string]any{
		"unknown":  {"idempotencyKey": uuid.New(), "text": "x", "photoId": other},
		"both":     {"idempotencyKey": uuid.New(), "text": "x", "photoId": pid, "photo": "AgAD"},
		"bad uuid": {"idempotencyKey": uuid.New(), "text": "x", "photoId": "nope"},
		"caption":  {"idempotencyKey": uuid.New(), "text": strings.Repeat("a", 1025), "photoId": pid},
	} {
		if w := send(h, http.MethodPost, "/api/broadcasts", c, jsonBody(t, body)); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := send(h, http.MethodPut, "/api/broadcasts/"+uuid.NewString(), c, jsonBody(t, map[string]any{"photoId": other})); w.Code != http.StatusBadRequest {
		t.Errorf("draft unknown photo: %d %s", w.Code, w.Body)
	}
	if w := send(h, http.MethodPost, "/api/broadcasts/preview-send", c, jsonBody(t, map[string]any{"text": "x", "photoId": other})); w.Code != http.StatusBadRequest {
		t.Errorf("preview unknown photo: %d", w.Code)
	}
}
