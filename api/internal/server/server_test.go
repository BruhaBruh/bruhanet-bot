package server_test

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/core/store"
)

const adminID = 1

var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

const indexHTML = "<!doctype html><title>test</title>"

// fakeStore повторяет семантику core/store в памяти.
type fakeStore struct {
	mu       sync.Mutex
	tokens   map[string]time.Time // hash → expires_at; удалённый = использованный
	sessions map[string]session
}

type session struct {
	id  int64
	exp time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{tokens: map[string]time.Time{}, sessions: map[string]session{}}
}

func (f *fakeStore) addToken(t *testing.T, exp time.Time) string {
	t.Helper()
	raw, hash, err := auth.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[hex.EncodeToString(hash)] = exp
	return raw
}

func (f *fakeStore) addSession(t *testing.T, id int64, exp time.Time) string {
	t.Helper()
	raw, hash, err := auth.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[hex.EncodeToString(hash)] = session{id: id, exp: exp}
	return raw
}

func (f *fakeStore) sessionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func (f *fakeStore) ConsumeLoginToken(_ context.Context, hash []byte, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := hex.EncodeToString(hash)
	exp, ok := f.tokens[k]
	if !ok || !exp.After(now) {
		return store.ErrTokenInvalid
	}
	delete(f.tokens, k)
	return nil
}

func (f *fakeStore) CreateSession(_ context.Context, hash []byte, id int64, exp time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[hex.EncodeToString(hash)] = session{id: id, exp: exp}
	return nil
}

func (f *fakeStore) SessionTelegramID(_ context.Context, hash []byte, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[hex.EncodeToString(hash)]
	if !ok || !s.exp.After(now) {
		return 0, store.ErrNotFound
	}
	return s.id, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, hex.EncodeToString(hash))
	return nil
}

func newServer(t *testing.T, baseURL string) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	h := server.New(server.Deps{
		Store:   st,
		AdminID: adminID,
		BaseURL: baseURL,
		Now:     func() time.Time { return now },
		Static: fstest.MapFS{
			"index.html":    {Data: []byte(indexHTML)},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
	})
	return h, st
}

func do(h http.Handler, method, target, cookie, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAPIRequiresSession(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")

	if w := do(h, http.MethodGet, "/api/me", "", ""); w.Code != http.StatusUnauthorized ||
		strings.TrimSpace(w.Body.String()) != `{"error":"unauthorized"}` {
		t.Fatalf("no cookie: %d %s", w.Code, w.Body)
	}
	if w := do(h, http.MethodGet, "/api/me", "garbage", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage cookie: %d", w.Code)
	}
	expired := st.addSession(t, adminID, now.Add(-time.Second))
	if w := do(h, http.MethodGet, "/api/me", expired, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired: %d", w.Code)
	}
	foreign := st.addSession(t, 42, now.Add(time.Hour))
	if w := do(h, http.MethodGet, "/api/me", foreign, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("foreign id: %d", w.Code)
	}

	valid := st.addSession(t, adminID, now.Add(time.Hour))
	w := do(h, http.MethodGet, "/api/me", valid, "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"telegramId":1}` ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("valid: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

// TestAPIDefaultDeny — /api/* закрыт сессией целиком, включая ещё не написанные и неизвестные маршруты.
func TestAPIDefaultDeny(t *testing.T) {
	h, _ := newServer(t, "http://localhost:5173")
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodPost, "/api/logout"},
		{http.MethodGet, "/api/"},
		{http.MethodGet, "/api/nope"},
		{http.MethodGet, "/api/logout"},
		{http.MethodDelete, "/api/users/1"},
	} {
		ct := ""
		if rt.method != http.MethodGet {
			ct = "application/json"
		}
		if w := do(h, rt.method, rt.path, "", ct); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without cookie: %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestJSONOnly(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	if w := do(h, http.MethodPost, "/api/logout", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/logout", c, ""); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("no content-type: %d", w.Code)
	}
	if st.sessionCount() != 1 {
		t.Fatal("rejected request must not log out")
	}
	if w := do(h, http.MethodPost, "/api/logout", c, "application/json; charset=utf-8"); w.Code != http.StatusNoContent {
		t.Fatalf("json: %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	w := do(h, http.MethodPost, "/api/logout", c, "application/json")
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	sc := w.Header().Get("Set-Cookie")
	if !strings.Contains(sc, "bn_session=") || !strings.Contains(sc, "Max-Age=0") {
		t.Fatalf("set-cookie = %q", sc)
	}
	if w := do(h, http.MethodGet, "/api/me", c, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", w.Code)
	}
}
