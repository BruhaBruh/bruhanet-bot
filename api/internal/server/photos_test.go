package server_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
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

func photoServer(t *testing.T) (http.Handler, *fakeTG, string) {
	t.Helper()
	h, st, tg := newServerTG(t, "http://localhost:5173", &fakePanel{}, 0)
	return h, tg, st.addSession(t, adminID, now.Add(time.Hour))
}

func TestPhotoUpload(t *testing.T) {
	h, tg, c := photoServer(t)
	img := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{1}, 1000)...)
	body, ct := multipartBody(t, "file", "a.png", img)

	w := upload(h, c, ct, body)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"photo":"AgAD-file"}` {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if tg.calls != 1 || tg.chatID != adminID || tg.caption != "Фото для рассылки" || tg.name != "a.png" || !bytes.Equal(tg.data, img) {
		t.Fatalf("tg = %d %d %q %q %d", tg.calls, tg.chatID, tg.caption, tg.name, len(tg.data))
	}

	tg.err = fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: PHOTO_INVALID_DIMENSIONS")
	body, ct = multipartBody(t, "file", "a.png", img)
	if w := upload(h, c, ct, body); w.Code != http.StatusBadRequest ||
		strings.TrimSpace(w.Body.String()) != `{"error":"invalid","message":"Telegram не принял фото: Bad Request: PHOTO_INVALID_DIMENSIONS"}` {
		t.Fatalf("telegram 400: %d %s", w.Code, w.Body)
	}

	tg.err = errors.New("boom")
	body, ct = multipartBody(t, "file", "a.png", img)
	if w := upload(h, c, ct, body); w.Code != http.StatusBadGateway ||
		strings.TrimSpace(w.Body.String()) != `{"error":"telegram_unavailable"}` {
		t.Fatalf("telegram down: %d %s", w.Code, w.Body)
	}
}

// Review Focus 5: не картинка или больше 10 МБ — 400 до обращения к Telegram.
func TestPhotoUploadRejects(t *testing.T) {
	h, tg, c := photoServer(t)
	big := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{1}, 10<<20)...)

	for _, tc := range []struct {
		name, field string
		data        []byte
		msg         string
	}{
		{"text", "file", []byte("hello, i am not an image"), "JPEG, PNG или WebP"},
		{"gif", "file", []byte("GIF89a......"), "JPEG, PNG или WebP"},
		{"big", "file", big, "файл больше 10 МБ"},
		{"no file", "other", pngHeader, "нет файла"},
	} {
		body, ct := multipartBody(t, tc.field, "x", tc.data)
		w := upload(h, c, ct, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.msg) {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body)
		}
	}
	if tg.calls != 0 {
		t.Fatalf("telegram called %d times", tg.calls)
	}
}

func TestPhotoUploadAuthAndType(t *testing.T) {
	h, tg, c := photoServer(t)
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
	if tg.calls != 1 {
		t.Fatalf("calls = %d", tg.calls)
	}
}
