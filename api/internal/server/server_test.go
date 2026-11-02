package server_test

import (
	"context"
	"encoding/hex"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
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

	users    []model.UserSummary
	payments []model.Payment // по возрастанию id
	revenue  int
	lastQ    string
	lastF    model.PaymentFilter
	lastRev  [2]time.Time

	overrides map[string]string
	setRaw    map[string][]byte
	outbox    []model.OutboxMessage
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

func (f *fakeStore) TextOverrides(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	maps.Copy(out, f.overrides)
	return out, nil
}

func (f *fakeStore) SetText(_ context.Context, key, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.overrides == nil {
		f.overrides = map[string]string{}
	}
	f.overrides[key] = body
	return nil
}

func (f *fakeStore) ResetText(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.overrides, key)
	return nil
}

func (f *fakeStore) SettingsRaw(context.Context) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]byte{}
	maps.Copy(out, f.setRaw)
	return out, nil
}

func (f *fakeStore) SetSettings(_ context.Context, v map[string][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setRaw == nil {
		f.setRaw = map[string][]byte{}
	}
	maps.Copy(f.setRaw, v)
	return nil
}

func (f *fakeStore) Enqueue(_ context.Context, m model.OutboxMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outbox = append(f.outbox, m)
	return nil
}

func newServer(t *testing.T, baseURL string) (http.Handler, *fakeStore) {
	t.Helper()
	return newServerPanel(t, baseURL, &fakePanel{})
}

func newServerPanel(t *testing.T, baseURL string, panel server.Panel) (http.Handler, *fakeStore) {
	t.Helper()
	return newServerTimeout(t, baseURL, panel, 0)
}

func newServerTimeout(t *testing.T, baseURL string, panel server.Panel, panelTimeout time.Duration) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	h := server.New(server.Deps{
		Store:        st,
		Panel:        panel,
		Defaults:     settings.Defaults(map[int]int{1: 150, 6: 800}, "+7 900", "Т-Банк", "https://t.me/support"),
		PanelTimeout: panelTimeout,
		AdminID:      adminID,
		BaseURL:      baseURL,
		Now:          func() time.Time { return now },
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

func (f *fakeStore) SearchUsers(_ context.Context, q string, limit int) ([]model.UserSummary, error) {
	f.lastQ = q
	if len(f.users) > limit {
		return f.users[:limit], nil
	}
	return f.users, nil
}

func (f *fakeStore) UserByID(_ context.Context, id int64) (model.UserSummary, error) {
	for _, u := range f.users {
		if u.TelegramID == id {
			return u, nil
		}
	}
	return model.UserSummary{}, store.ErrNotFound
}

func (f *fakeStore) PaymentsByUser(_ context.Context, id int64) ([]model.Payment, error) {
	out := []model.Payment{}
	for i := len(f.payments) - 1; i >= 0; i-- {
		if f.payments[i].TelegramID == id {
			out = append(out, f.payments[i])
		}
	}
	return out, nil
}

func (f *fakeStore) UserStats(_ context.Context, id, _ int64) (model.UserStats, error) {
	return model.UserStats{Confirmed: 1, ConfirmedSum: 150}, nil
}

func (f *fakeStore) ListPayments(_ context.Context, fl model.PaymentFilter) ([]model.Payment, int, error) {
	f.lastF = fl
	return f.payments, len(f.payments), nil
}

func (f *fakeStore) Revenue(_ context.Context, from, to time.Time) (int, error) {
	f.lastRev = [2]time.Time{from, to}
	return f.revenue, nil
}

type fakePanel struct {
	user *api.UserItemInfo
	err  error
}

func (p *fakePanel) GetUser(context.Context, int64) (*api.UserItemInfo, error) { return p.user, p.err }

// blockingPanel висит, пока не отменят контекст.
type blockingPanel struct{}

func (blockingPanel) GetUser(ctx context.Context, _ int64) (*api.UserItemInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
