package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthOK(t *testing.T) {
	for _, tc := range []struct {
		baseURL string
		secure  bool
	}{
		{"http://localhost:5173", false},
		{"https://admin.bruhanet.ru/", true},
	} {
		t.Run(tc.baseURL, func(t *testing.T) {
			h, st := newServer(t, tc.baseURL)
			tok := st.addToken(t, now.Add(5*time.Minute))

			w := do(h, http.MethodGet, "/auth?token="+tok, "", "")
			if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
				t.Fatalf("auth: %d %q", w.Code, w.Header().Get("Location"))
			}
			assertNoLeak(t, w)
			sc := w.Header().Get("Set-Cookie")
			for _, part := range []string{"bn_session=", "Path=/", "HttpOnly", "SameSite=Strict"} {
				if !strings.Contains(sc, part) {
					t.Fatalf("set-cookie %q lacks %q", sc, part)
				}
			}
			if strings.Contains(sc, "Secure") != tc.secure {
				t.Fatalf("set-cookie %q: secure want %v", sc, tc.secure)
			}

			cookie := w.Result().Cookies()[0].Value
			if cookie == tok {
				t.Fatal("session must not reuse the login token")
			}
			if w := do(h, http.MethodGet, "/api/me", cookie, ""); w.Code != http.StatusOK {
				t.Fatalf("me with new session: %d", w.Code)
			}
		})
	}
}

// assertNoLeak — токен в URL не должен утечь через Referer или кэш.
func assertNoLeak(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if rp, cc := w.Header().Get("Referrer-Policy"), w.Header().Get("Cache-Control"); rp != "no-referrer" || cc != "no-store" {
		t.Fatalf("Referrer-Policy = %q, Cache-Control = %q", rp, cc)
	}
}

func assertStale(t *testing.T, h http.Handler, target string) {
	t.Helper()
	w := do(h, http.MethodGet, target, "", "")
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "Ссылка устарела") ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%s: %d %s %s", target, w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	assertNoLeak(t, w)
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("stale link must not set a cookie")
	}
}

func TestAuthReusedToken(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	tok := st.addToken(t, now.Add(5*time.Minute))
	if w := do(h, http.MethodGet, "/auth?token="+tok, "", ""); w.Code != http.StatusSeeOther {
		t.Fatalf("first use: %d", w.Code)
	}
	assertStale(t, h, "/auth?token="+tok)
	if n := st.sessionCount(); n != 1 {
		t.Fatalf("sessions = %d, want 1", n)
	}
}

func TestAuthExpired(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	tok := st.addToken(t, now)
	assertStale(t, h, "/auth?token="+tok)
	assertStale(t, h, "/auth?token=garbage")
	assertStale(t, h, "/auth")
	if n := st.sessionCount(); n != 0 {
		t.Fatalf("sessions = %d, want 0", n)
	}
}
