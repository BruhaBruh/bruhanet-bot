package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
)

const miniAdmin = `{"id":1,"first_name":"Админ","username":"boss"}`

func newMiniAdmin(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	st.users = []model.UserSummary{{TelegramID: 42, Username: "bruha"}, {TelegramID: adminID, Username: "boss"}}
	h := server.NewMiniApp(server.Deps{
		Store: st, Panel: &fakePanel{}, Defaults: settings.Defaults(map[int]int{1: 150}, "+7", "Б", "https://t.me/s"),
		BotToken: miniToken, MiniOrigin: miniOrigin, AdminID: adminID, BaseURL: "https://admin.example.test/",
		Now: func() time.Time { return now }, MiniStatic: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}},
	})
	return h, st
}

func adminLink(t *testing.T, h http.Handler, user string, age time.Duration) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/m/api/admin/login-link", strings.NewReader("{}"))
	r.Header.Set("Authorization", tma(t, miniToken, age, user))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", miniOrigin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMiniMeKnowsTheAdmin(t *testing.T) {
	h, _ := newMiniAdmin(t)
	for user, want := range map[string]bool{miniAdmin: true, miniUser: false} {
		w := miniReq(h, http.MethodGet, "/m/api/me", tma(t, miniToken, time.Minute, user), nil)
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		if m["isAdmin"] != want {
			t.Fatalf("isAdmin = %v, want %v (%s)", m["isAdmin"], want, w.Body.String())
		}
	}
}

func TestMiniAdminLoginLink(t *testing.T) {
	h, st := newMiniAdmin(t)
	w := adminLink(t, h, miniAdmin, time.Minute)
	var m map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	const prefix = "https://admin.example.test/auth?token="
	if w.Code != 200 || !strings.HasPrefix(m["url"], prefix) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s cc=%q", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
	}
	// ссылка рабочая и одноразовая: тот же токен, что гасит /auth
	token := strings.TrimPrefix(m["url"], prefix)
	if err := st.ConsumeLoginToken(context.Background(), auth.Hash(token), now); err != nil {
		t.Fatalf("token not accepted by /auth: %v", err)
	}
	if err := st.ConsumeLoginToken(context.Background(), auth.Hash(token), now); err == nil {
		t.Fatal("token must be single-use")
	}
}

func TestMiniAdminLoginLinkIsAdminOnly(t *testing.T) {
	h, st := newMiniAdmin(t)
	w := adminLink(t, h, miniUser, time.Minute)
	if w.Code != 404 || strings.Contains(w.Body.String(), "token") {
		t.Fatalf("a regular user must get a plain 404, got %d %s", w.Code, w.Body.String())
	}
	if len(st.tokens) != 0 {
		t.Fatalf("no token may be created for a non-admin: %d", len(st.tokens))
	}
}

// Ссылка на вход в админку — доступ ко всему: подпись для неё должна быть свежее общих двух часов.
func TestMiniAdminLoginLinkNeedsFreshSignature(t *testing.T) {
	h, st := newMiniAdmin(t)
	if w := adminLink(t, h, miniAdmin, 45*time.Minute); w.Code != 401 || !strings.Contains(w.Body.String(), "session_expired") {
		t.Fatalf("old signature: %d %s", w.Code, w.Body.String())
	}
	if len(st.tokens) != 0 {
		t.Fatal("no token for an old signature")
	}
	if w := adminLink(t, h, miniAdmin, 20*time.Minute); w.Code != 200 {
		t.Fatalf("20 minutes is fine: %d", w.Code)
	}
}

func TestMiniAdminLoginLinkRateLimit(t *testing.T) {
	h, _ := newMiniAdmin(t)
	for i := range 5 {
		if w := adminLink(t, h, miniAdmin, time.Minute); w.Code != 200 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	if w := adminLink(t, h, miniAdmin, time.Minute); w.Code != 429 {
		t.Fatalf("6th link in a minute must be limited, got %d", w.Code)
	}
}
