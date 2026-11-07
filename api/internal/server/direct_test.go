package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

func TestDirectMessage(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{err: errors.New("panel down")})

	if w := send(h, http.MethodPost, "/api/users/8/messages", c, `{"text":"Привет"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"text":""}`, `{"text":"<i>x"}`, `nope`} {
		if w := send(h, http.MethodPost, "/api/users/7/messages", c, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := send(h, http.MethodPost, "/api/users/x/messages", c, `{"text":"a"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
	if len(st.directs) != 0 {
		t.Fatalf("directs = %+v", st.directs)
	}

	w := send(h, http.MethodPost, "/api/users/7/messages", c, `{"text":"<b>Привет</b>","photo":"AgAD"}`)
	if w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != `{"id":1}` {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if d := st.directs[0]; d.TelegramID != 7 || d.Text != "<b>Привет</b>" || d.Photo != "AgAD" {
		t.Fatalf("direct = %+v", d)
	}

	var out struct {
		Messages []model.DirectMessage `json:"messages"`
	}
	getJSON(t, h, "/api/users/7", c, &out)
	if len(out.Messages) != 1 || out.Messages[0].ID != 1 || out.Messages[0].Status != model.DirectPending {
		t.Fatalf("messages = %+v", out.Messages)
	}
}

// Личное сообщение с загруженной картинкой: photoId уходит в store (и дальше в outbox), неизвестный — 400.
func TestDirectMessagePhotoID(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	pid, _ := st.SavePhoto(t.Context(), []byte("IMG"), "image/png")

	w := send(h, http.MethodPost, "/api/users/7/messages", c, `{"text":"","photoId":"`+pid.String()+`"}`)
	if w.Code != http.StatusCreated || len(st.directs) != 1 || st.directs[0].PhotoID == nil || *st.directs[0].PhotoID != pid {
		t.Fatalf("%d %s %+v", w.Code, w.Body, st.directs)
	}
	if w := send(h, http.MethodPost, "/api/users/7/messages", c, `{"text":"x","photoId":"`+uuid.NewString()+`"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown photo: %d %s", w.Code, w.Body)
	}
}

func getJSON(t *testing.T, h http.Handler, target, c string, v any) {
	t.Helper()
	w := do(h, http.MethodGet, target, c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", target, w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatal(err)
	}
}
