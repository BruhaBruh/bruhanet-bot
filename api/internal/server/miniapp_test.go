package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
)

const (
	miniToken  = "123456:TEST-TOKEN"
	miniOrigin = "https://app.example.test"
	miniUser   = `{"id":42,"first_name":"Бруха","username":"bruha"}`
)

func newMini(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	h, st, _ := newMiniPanel(t, &fakePanel{})
	return h, st
}

func newMiniPanel(t *testing.T, panel *fakePanel) (http.Handler, *fakeStore, *fakeIdentity) {
	t.Helper()
	st := newFakeStore()
	st.users = []model.UserSummary{{TelegramID: 42, Username: "bruha", Name: "Бруха"}}
	id := &fakeIdentity{botUser: "bruhanet_bot", botName: "BruhaNet"}
	h := server.NewMiniApp(server.Deps{
		Store:      st,
		Panel:      panel,
		Identity:   id,
		Defaults:   settings.Defaults(map[int]int{1: 150}, "+7 900", "Т-Банк", "https://t.me/support"),
		BotToken:   miniToken,
		MiniOrigin: miniOrigin,
		Now:        func() time.Time { return now },
		MiniStatic: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}},
	})
	return h, st, id
}

func miniReq(h http.Handler, method, target, auth string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func tma(t *testing.T, token string, age time.Duration, user string) string {
	t.Helper()
	return "tma " + server.SignInitData(t, token, now.Add(-age), user)
}

func TestMiniAuth(t *testing.T) {
	h, _ := newMini(t)
	tests := []struct {
		name string
		auth string
		code int
		body string
	}{
		{"no header", "", 401, "unauthorized"},
		{"wrong scheme", "Bearer " + strings.TrimPrefix(tma(t, miniToken, time.Minute, miniUser), "tma "), 401, "unauthorized"},
		{"foreign bot token", tma(t, "999:OTHER", time.Minute, miniUser), 401, "unauthorized"},
		{"expired", tma(t, miniToken, 3*time.Hour, miniUser), 401, "session_expired"},
		{"not registered", tma(t, miniToken, time.Minute, `{"id":777}`), 403, "not_registered"},
		{"ok", tma(t, miniToken, time.Minute, miniUser), 200, `"username":"bruha"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := miniReq(h, http.MethodGet, "/m/api/me", tc.auth, nil)
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("got %d %s, want %d containing %q", w.Code, w.Body.String(), tc.code, tc.body)
			}
		})
	}
}

func TestMiniHeaders(t *testing.T) {
	h, _ := newMini(t)
	w := miniReq(h, http.MethodGet, "/m/api/me", tma(t, miniToken, time.Minute, miniUser), nil)
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	// заголовки безопасности и на статике, и на 401
	for _, w := range []*httptest.ResponseRecorder{w, miniReq(h, http.MethodGet, "/", "", nil), miniReq(h, http.MethodGet, "/m/api/me", "", nil)} {
		csp := w.Header().Get("Content-Security-Policy")
		for _, want := range []string{"script-src 'self' https://telegram.org", "frame-ancestors https://web.telegram.org https://*.telegram.org", "object-src 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("csp missing %q: %s", want, csp)
			}
		}
		if strings.Contains(csp, "unsafe-eval") {
			t.Errorf("unsafe-eval in csp: %s", csp)
		}
		if w.Header().Get("X-Frame-Options") != "" {
			t.Error("X-Frame-Options must not be set")
		}
		if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Error("referrer/nosniff headers missing")
		}
	}
	if w := miniReq(h, http.MethodGet, "/m/api/me", "", nil); w.Header().Get("Cache-Control") != "no-store" {
		t.Error("401 on /m/api must also be no-store")
	}
}

func TestMiniAdminRoutesAbsent(t *testing.T) {
	h, st := newMini(t)
	cookie := st.addSession(t, adminID, now.Add(time.Hour))
	for _, p := range []string{"/api/users", "/auth", "/auth/bot"} {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, p, nil)
		r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		// SPA-фолбэк отдаёт index.html, но не данные админки
		if strings.Contains(w.Body.String(), "telegramId") || w.Code == http.StatusSeeOther {
			t.Errorf("%s leaked admin behavior: %d %s", p, w.Code, w.Body.String())
		}
	}
}

func TestMiniMutationGuard(t *testing.T) {
	h, _ := newMini(t)
	auth := tma(t, miniToken, time.Minute, miniUser)
	json := map[string]string{"Content-Type": "application/json"}
	cases := []struct {
		name string
		hdr  map[string]string
		code int
	}{
		{"foreign origin", map[string]string{"Content-Type": "application/json", "Origin": "https://evil.test"}, 403},
		{"no origin", json, 403},
		{"not json", map[string]string{"Content-Type": "text/plain", "Origin": miniOrigin}, 415},
		{"ok reaches router", map[string]string{"Content-Type": "application/json", "Origin": miniOrigin}, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if w := miniReq(h, http.MethodPost, "/m/api/nothing", auth, tc.hdr); w.Code != tc.code {
				t.Fatalf("got %d, want %d (%s)", w.Code, tc.code, w.Body.String())
			}
		})
	}
}
