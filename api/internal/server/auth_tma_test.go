package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/settings"
)

const tmaBase = "https://admin.example.test"

func newServerTMA(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	h := server.New(server.Deps{
		Store: st, Panel: &fakePanel{}, Defaults: settings.Defaults(map[int]int{1: 150}, "+7", "Б", "https://t.me/s"),
		AdminID: adminID, BaseURL: tmaBase, BotToken: miniToken, Now: func() time.Time { return now },
		Static: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}},
	})
	return h, st
}

func tmaLogin(h http.Handler, initData, origin, contentType string) *httptest.ResponseRecorder {
	body := `{"initData":` + jsonString(initData) + `}`
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/tma", strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func jsonString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func adminInit(t *testing.T, age time.Duration, id string) string {
	t.Helper()
	return server.SignInitData(t, miniToken, now.Add(-age), `{"id":`+id+`,"first_name":"A"}`)
}

func TestAuthTMAAdminGetsSession(t *testing.T) {
	h, st := newServerTMA(t)
	w := tmaLogin(h, adminInit(t, 4*time.Minute, "1"), tmaBase, "application/json")
	if w.Code != 204 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != "bn_session" || !c[0].HttpOnly || !c[0].Secure || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", c)
	}
	if st.sessionCount() != 1 {
		t.Fatalf("sessions = %d", st.sessionCount())
	}
	if w := do(h, http.MethodGet, "/api/me", c[0].Value, ""); w.Code != 200 {
		t.Fatalf("the cookie must open the admin api, got %d", w.Code)
	}
}

func TestAuthTMARefusals(t *testing.T) {
	h, st := newServerTMA(t)
	good := adminInit(t, time.Minute, "1")
	other := url.Values{}
	other.Set("x", "y")
	cases := []struct {
		name, init, origin, ct string
		code                   int
	}{
		{"not the admin", adminInit(t, time.Minute, "42"), tmaBase, "application/json", 401},
		{"older than 5 minutes", adminInit(t, 6*time.Minute, "1"), tmaBase, "application/json", 401},
		{"other bot token", server.SignInitData(t, "999:OTHER", now.Add(-time.Minute), `{"id":1}`), tmaBase, "application/json", 401},
		{"garbage", "hash=00", tmaBase, "application/json", 401},
		{"foreign origin", good, "https://evil.test", "application/json", 403},
		{"no origin", good, "", "application/json", 403},
		{"not json", good, tmaBase, "text/plain", 415},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := tmaLogin(h, tc.init, tc.origin, tc.ct)
			if w.Code != tc.code || len(w.Result().Cookies()) != 0 {
				t.Fatalf("got %d cookies=%v", w.Code, w.Result().Cookies())
			}
		})
	}
	if st.sessionCount() != 0 {
		t.Fatalf("no session may be created, got %d", st.sessionCount())
	}
}

func TestAuthTMARateLimit(t *testing.T) {
	h, _ := newServerTMA(t)
	bad := adminInit(t, time.Minute, "42")
	for i := range 20 {
		if w := tmaLogin(h, bad, tmaBase, "application/json"); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	if w := tmaLogin(h, bad, tmaBase, "application/json"); w.Code != 429 {
		t.Fatalf("21st attempt in a minute must be limited, got %d", w.Code)
	}
}

// Подпись одноразовая: перехваченный initData из Mini App нельзя предъявить второй раз.
func TestAuthTMAInitDataIsSingleUse(t *testing.T) {
	h, st := newServerTMA(t)
	init := adminInit(t, time.Minute, "1")
	if w := tmaLogin(h, init, "https://evil.test", "application/json"); w.Code != 403 {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	if w := tmaLogin(h, init, tmaBase, "application/json"); w.Code != 204 {
		t.Fatalf("a refused request must not burn the signature, got %d", w.Code)
	}
	if w := tmaLogin(h, init, tmaBase, "application/json"); w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("replay must be refused, got %d", w.Code)
	}
	if st.sessionCount() != 1 {
		t.Fatalf("sessions = %d", st.sessionCount())
	}
	// другой запуск Mini App (другая подпись) проходит
	if w := tmaLogin(h, server.SignInitData(t, miniToken, now.Add(-30*time.Second), `{"id":1,"first_name":"A"}`, "query_id", "AAZ"), tmaBase, "application/json"); w.Code != 204 {
		t.Fatalf("a new launch must pass, got %d", w.Code)
	}
}

// Лимит считается по юзеру из подписи: мусор и чужой юзер не могут заблокировать вход админа.
func TestAuthTMALimitIsPerSignedUser(t *testing.T) {
	h, _ := newServerTMA(t)
	for range 60 {
		tmaLogin(h, "hash=00", tmaBase, "application/json") // подпись невалидна: ничего не тратит
	}
	other := adminInit(t, time.Minute, "42")
	for range 25 {
		tmaLogin(h, other, tmaBase, "application/json") // 42 выбил свой лимит
	}
	if w := tmaLogin(h, other, tmaBase, "application/json"); w.Code != 429 {
		t.Fatalf("user 42 must be limited, got %d", w.Code)
	}
	if w := tmaLogin(h, adminInit(t, time.Minute, "1"), tmaBase, "application/json"); w.Code != 204 {
		t.Fatalf("the admin must still log in, got %d", w.Code)
	}
}
