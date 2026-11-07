package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/model"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

const subURL = "https://sub.example/SECRET-token"

func devPanel() *fakePanel {
	return &fakePanel{
		user: &api.UserItemInfo{ID: 42, Status: api.UserItemInfoStatusACTIVE, ExpireAt: now, SubscriptionUrl: subURL, HwidDeviceLimit: api.NewNilInt(5)},
		devices: []api.Device{
			{Hwid: "h1", Platform: api.NewNilString("ios"), UserAgent: api.NewNilString("Happ/1"), CreatedAt: now, UpdatedAt: now},
			{Hwid: "h2", Platform: api.NilString{Null: true}, OsVersion: api.NilString{Null: true}, DeviceModel: api.NilString{Null: true}, UserAgent: api.NilString{Null: true}, CreatedAt: now, UpdatedAt: now},
		},
	}
}

func TestDevicesRequireSession(t *testing.T) {
	h, _, _ := cardServer(t, devPanel())
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		if w := do(h, m, "/api/users/7/devices", "", "application/json"); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices/h1", "", "application/json"); w.Code != http.StatusUnauthorized {
		t.Fatalf("delete one: %d", w.Code)
	}
}

func TestDevicesList(t *testing.T) {
	p := devPanel()
	h, _, c := cardServer(t, p)
	w := do(h, http.MethodGet, "/api/users/7/devices", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, want := range []string{`"limit":5`, `"hwid":"h1"`, `"platform":"ios"`, `"osVersion":null`, `"userAgent":null`} {
		if !strings.Contains(body, want) {
			t.Fatalf("no %s in %s", want, body)
		}
	}
	if strings.Contains(body, "SECRET") || strings.Contains(body, "requestIp") {
		t.Fatalf("leak: %s", body)
	}

	if w := do(h, http.MethodGet, "/api/users/99/devices", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if w := do(h, http.MethodGet, "/api/users/x/devices", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestDevicesEmptyAndFailures(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{err: remnawave.ErrUserNotFound})
	w := do(h, http.MethodGet, "/api/users/7/devices", c, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) || !strings.Contains(w.Body.String(), `"limit":null`) {
		t.Fatalf("not in panel: %d %s", w.Code, w.Body)
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices", c, "application/json"); w.Code != http.StatusNotFound {
		t.Fatalf("delete not in panel: %d", w.Code)
	}

	h, _, c = cardServer(t, &fakePanel{err: errors.New("boom")})
	if w := do(h, http.MethodGet, "/api/users/7/devices", c, ""); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "panel_unavailable") {
		t.Fatalf("get user fail: %d %s", w.Code, w.Body)
	}

	p := devPanel()
	p.devErr = errors.New("boom")
	h, _, c = cardServer(t, p)
	if w := do(h, http.MethodGet, "/api/users/7/devices", c, ""); w.Code != http.StatusBadGateway {
		t.Fatalf("list fail: %d", w.Code)
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices/h1", c, "application/json"); w.Code != http.StatusBadGateway {
		t.Fatalf("delete fail: %d", w.Code)
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices", c, "application/json"); w.Code != http.StatusBadGateway {
		t.Fatalf("delete all fail: %d", w.Code)
	}

	h, st := newServerTimeout(t, "http://localhost:5173", &hangDevices{devPanel()}, 50*time.Millisecond)
	st.users = []model.UserSummary{{TelegramID: 7}}
	c = st.addSession(t, adminID, now.Add(time.Hour))
	if w := do(h, http.MethodGet, "/api/users/7/devices", c, ""); w.Code != http.StatusBadGateway {
		t.Fatalf("timeout: %d", w.Code)
	}
}

func TestDevicesDelete(t *testing.T) {
	p := devPanel()
	h, _, c := cardServer(t, p)
	esc := url.PathEscape("a/b c")
	if w := do(h, http.MethodDelete, "/api/users/7/devices/"+esc, c, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices", c, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("delete all: %d", w.Code)
	}
	if got := strings.Join(p.deleted, ","); got != "42:a/b c,42:*" {
		t.Fatalf("deleted = %q", got)
	}
	if w := do(h, http.MethodDelete, "/api/users/99/devices/h1", c, "application/json"); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}

	p.devErr = remnawave.ErrDeviceNotFound
	if w := do(h, http.MethodDelete, "/api/users/7/devices/h1", c, "application/json"); w.Code != http.StatusNotFound {
		t.Fatalf("panel not found: %d", w.Code)
	}
}

func TestDevicesHwidValidation(t *testing.T) {
	p := devPanel()
	h, _, c := cardServer(t, p)
	for _, bad := range []string{strings.Repeat("a", 257), "a%00b", "a%0Ab", "%FF"} {
		if w := do(h, http.MethodDelete, "/api/users/7/devices/"+bad, c, "application/json"); w.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d", bad, w.Code)
		}
	}
	if len(p.deleted) != 0 {
		t.Fatalf("panel called: %v", p.deleted)
	}
	if w := do(h, http.MethodDelete, "/api/users/7/devices/"+strings.Repeat("a", 256), c, "application/json"); w.Code != http.StatusNoContent {
		t.Fatalf("256: %d", w.Code)
	}
}

func TestSubscriptionURLOnlyInCard(t *testing.T) {
	h, _, c := cardServer(t, devPanel())
	w := do(h, http.MethodGet, "/api/users/7", c, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"subscriptionUrl":"`+subURL+`"`) {
		t.Fatalf("card: %d %s", w.Code, w.Body)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	for _, path := range []string{"/api/users", "/api/users?tab=active", "/api/users/7/devices", "/api/payments", "/api/overview"} {
		w := do(h, http.MethodGet, path, c, "")
		if strings.Contains(w.Body.String(), "SECRET") || strings.Contains(strings.ToLower(w.Body.String()), "subscriptionurl") {
			t.Fatalf("%s leaks the link: %s", path, w.Body)
		}
	}
	// панель недоступна — ссылки нет
	h, _, c = cardServer(t, &fakePanel{err: errors.New("boom")})
	if w := do(h, http.MethodGet, "/api/users/7", c, ""); strings.Contains(w.Body.String(), "subscriptionUrl") {
		t.Fatalf("link without panel: %s", w.Body)
	}
}

// hangDevices: GetUser отвечает сразу, устройства висят до таймаута.
type hangDevices struct{ *fakePanel }

func (p *hangDevices) UserDevices(ctx context.Context, _ int) ([]api.Device, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
