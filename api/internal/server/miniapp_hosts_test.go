package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

const (
	keyAuto = "vless://11111111-2222-4333-8444-555555555555@h.example:443?type=tcp#%E2%9A%A1%EF%B8%8F%20%D0%90%D0%B2%D1%82%D0%BE"
	keyHy2  = "hysteria2://s3cretpass@h.example:8443#%F0%9F%87%A9%F0%9F%87%AA%20Hysteria%202"
	keyBare = "vless://aaaaaaaa-0000-4000-8000-000000000000@h.example:443"
)

func hostPanel() *fakePanel {
	p := &fakePanel{user: activeUser(now.Add(24 * time.Hour))}
	p.keys = []string{keyAuto, keyHy2, keyBare}
	return p
}

type hostItem struct{ ID, Name, Protocol string }

func listHosts(t *testing.T, h http.Handler) (int, []hostItem, string) {
	t.Helper()
	code, _, raw := getMini(t, h, http.MethodGet, "/m/api/hosts", "")
	var out struct{ Items []hostItem }
	_ = json.Unmarshal([]byte(strings.NewReplacer(`"id"`, `"ID"`, `"name"`, `"Name"`, `"protocol"`, `"Protocol"`).Replace(raw)), &out)
	return code, out.Items, raw
}

func hostLink(t *testing.T, h http.Handler, id string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/m/api/hosts/link", strings.NewReader(`{"id":"`+id+`"}`))
	r.Header.Set("Authorization", tma(t, miniToken, time.Minute, miniUser))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", miniOrigin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMiniHostsList(t *testing.T) {
	h, _, _ := newMiniPanel(t, hostPanel())
	code, items, raw := listHosts(t, h)
	if code != 200 || len(items) != 3 {
		t.Fatalf("%d %s", code, raw)
	}
	if items[0].Name != "⚡️ Авто" || items[0].Protocol != "vless" || items[1].Name != "🇩🇪 Hysteria 2" || items[1].Protocol != "hysteria2" || items[2].Name != "Ключ 3" {
		t.Fatalf("items = %+v", items)
	}
	idRe := regexp.MustCompile(`^[0-9a-f]{16}$`)
	seen := map[string]bool{}
	for _, it := range items {
		if !idRe.MatchString(it.ID) || seen[it.ID] {
			t.Fatalf("bad or duplicate id %q", it.ID)
		}
		seen[it.ID] = true
	}
	// в списке нет ни ссылок, ни их секретов
	for _, leak := range []string{"vless://", "hysteria2://", "11111111-2222", "s3cretpass", "h.example"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("list leaks %q: %s", leak, raw)
		}
	}
}

func TestMiniHostsListStates(t *testing.T) {
	h, _, _ := newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	if code, items, _ := listHosts(t, h); code != 200 || len(items) != 0 {
		t.Fatalf("not in panel: %d %v", code, items)
	}
	p := hostPanel()
	p.keysErr = http.ErrHandlerTimeout
	h, _, _ = newMiniPanel(t, p)
	if code, _, raw := listHosts(t, h); code != 502 || !strings.Contains(raw, "panel_unavailable") || strings.Contains(raw, "vless") {
		t.Fatalf("panel down: %d %s", code, raw)
	}
}

func TestMiniHostLink(t *testing.T) {
	h, _, _ := newMiniPanel(t, hostPanel())
	_, items, _ := listHosts(t, h)
	w := hostLink(t, h, items[1].ID)
	var m map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || m["url"] != keyHy2 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s cc=%q", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
	}
	if w := hostLink(t, h, "0123456789abcdef"); w.Code != 404 {
		t.Fatalf("unknown id: %d", w.Code)
	}
	for _, bad := range []string{"", "xyz", "0123456789ABCDEF", "0123456789abcde", "../../etc/passwd"} {
		if w := hostLink(t, h, bad); w.Code != 400 {
			t.Fatalf("id %q: %d", bad, w.Code)
		}
	}
}

func TestMiniHostLinkNoPanelUserAndRateLimit(t *testing.T) {
	h, _, _ := newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	if w := hostLink(t, h, "0123456789abcdef"); w.Code != 404 {
		t.Fatalf("not in panel: %d", w.Code)
	}
	h, _, _ = newMiniPanel(t, hostPanel())
	_, items, _ := listHosts(t, h)
	for i := range 40 {
		if w := hostLink(t, h, items[0].ID); w.Code != 200 {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
	if w := hostLink(t, h, items[0].ID); w.Code != 429 {
		t.Fatalf("41st link in a minute must be limited, got %d", w.Code)
	}
}
