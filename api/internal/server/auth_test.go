package server_test

import (
	"net/http"
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

func assertStale(t *testing.T, h http.Handler, target string) {
	t.Helper()
	w := do(h, http.MethodGet, target, "", "")
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "Ссылка устарела") ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%s: %d %s %s", target, w.Code, w.Header().Get("Content-Type"), w.Body)
	}
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
