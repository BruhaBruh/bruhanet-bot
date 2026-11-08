package server_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

const miniSubURL = "https://sub.example.test/SECRET-SHORT-TOKEN"

func activeUser(expire time.Time) *api.UserItemInfo {
	return &api.UserItemInfo{
		ID: 7, ShortUuid: "SECRETSHORT", Status: api.UserItemInfoStatusACTIVE, ExpireAt: expire, SubscriptionUrl: miniSubURL,
		TrafficLimitBytes: 100 << 30, HwidDeviceLimit: api.NewNilInt(5),
		UserTraffic: api.UserTrafficItem{UsedTrafficBytes: 10 << 30},
	}
}

func getMini(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	hdr := map[string]string{}
	if method != http.MethodGet {
		hdr["Content-Type"] = "application/json"
		hdr["Origin"] = miniOrigin
	}
	w := miniReq(h, method, path, tma(t, miniToken, time.Minute, miniUser), hdr)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	_ = body
	return w.Code, m, w.Body.String()
}

func TestMiniSubscription(t *testing.T) {
	t.Run("active, no link in the body", func(t *testing.T) {
		panel := &fakePanel{user: activeUser(now.Add(10*24*time.Hour + time.Hour)), devices: []api.Device{{Hwid: "a"}, {Hwid: "b"}}}
		h, _, _ := newMiniPanel(t, panel)
		code, m, raw := getMini(t, h, http.MethodGet, "/m/api/subscription", "")
		if code != 200 || m["state"] != "active" || m["daysLeft"] != float64(11) {
			t.Fatalf("%d %s", code, raw)
		}
		for _, leak := range []string{miniSubURL, "SECRET", "subscriptionUrl", "shortUuid"} {
			if strings.Contains(raw, leak) {
				t.Fatalf("body leaks %q: %s", leak, raw)
			}
		}
		d := m["devices"].(map[string]any)
		if d["used"] != float64(2) || d["limit"] != float64(5) {
			t.Fatalf("devices = %v", d)
		}
		if m["trafficUsedBytes"] != float64(10<<30) || m["trafficLimitBytes"] != float64(100<<30) {
			t.Fatalf("traffic = %v", m)
		}
	})
	t.Run("unlimited traffic is null", func(t *testing.T) {
		u := activeUser(now.Add(24 * time.Hour))
		u.TrafficLimitBytes = 0
		h, _, _ := newMiniPanel(t, &fakePanel{user: u})
		if _, m, raw := getMini(t, h, http.MethodGet, "/m/api/subscription", ""); m["trafficLimitBytes"] != nil {
			t.Fatalf("limit not null: %s", raw)
		}
	})
	t.Run("expired and disabled", func(t *testing.T) {
		h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(-time.Hour))})
		if _, m, _ := getMini(t, h, http.MethodGet, "/m/api/subscription", ""); m["state"] != "expired" || m["daysLeft"] != float64(0) {
			t.Fatalf("state = %v", m)
		}
		u := activeUser(now.Add(time.Hour))
		u.Status = api.UserItemInfoStatusDISABLED
		h, _, _ = newMiniPanel(t, &fakePanel{user: u})
		if _, m, _ := getMini(t, h, http.MethodGet, "/m/api/subscription", ""); m["state"] != "disabled" {
			t.Fatalf("state = %v", m)
		}
	})
	t.Run("not in panel", func(t *testing.T) {
		h, _, _ := newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
		if code, m, raw := getMini(t, h, http.MethodGet, "/m/api/subscription", ""); code != 200 || m["state"] != "none" || m["expireAt"] != nil {
			t.Fatalf("%d %s", code, raw)
		}
	})
	t.Run("panel down is 502", func(t *testing.T) {
		h, _, _ := newMiniPanel(t, &fakePanel{err: http.ErrHandlerTimeout})
		if code, m, raw := getMini(t, h, http.MethodGet, "/m/api/subscription", ""); code != 502 || m["error"] != "panel_unavailable" || strings.Contains(raw, "SECRET") {
			t.Fatalf("%d %s", code, raw)
		}
	})
	t.Run("devices list failure keeps the rest", func(t *testing.T) {
		h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(24 * time.Hour)), devErr: http.ErrHandlerTimeout})
		code, m, _ := getMini(t, h, http.MethodGet, "/m/api/subscription", "")
		if d := m["devices"].(map[string]any); code != 200 || d["used"] != nil || d["limit"] != float64(5) {
			t.Fatalf("%d %v", code, m)
		}
	})
}

func TestMiniSubscriptionLink(t *testing.T) {
	h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(24 * time.Hour))})
	if code, _, _ := getMini(t, h, http.MethodGet, "/m/api/subscription-link", ""); code != 405 && code != 404 {
		t.Fatalf("GET must not return the link, got %d", code)
	}
	w := miniReq(h, http.MethodPost, "/m/api/subscription-link", tma(t, miniToken, time.Minute, miniUser),
		map[string]string{"Content-Type": "application/json", "Origin": miniOrigin})
	var m map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || m["url"] != miniSubURL || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s cc=%q", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
	}
	h, _, _ = newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	w = miniReq(h, http.MethodPost, "/m/api/subscription-link", tma(t, miniToken, time.Minute, miniUser),
		map[string]string{"Content-Type": "application/json", "Origin": miniOrigin})
	if w.Code != 404 {
		t.Fatalf("no panel user: %d", w.Code)
	}
}

func TestMiniReferral(t *testing.T) {
	h, st, _ := newMiniPanel(t, &fakePanel{})
	// выключена (referralDays = 0 по умолчанию)
	if _, m, raw := getMini(t, h, http.MethodGet, "/m/api/referral", ""); m["enabled"] != false {
		t.Fatalf("must be disabled: %s", raw)
	}
	st.setRaw = map[string][]byte{"referralDays": []byte("7")}
	st.referral = model.UserReferral{Invited: 5, Paid: 2}
	st.rewardDays = 14
	_, m, raw := getMini(t, h, http.MethodGet, "/m/api/referral", "")
	if m["enabled"] != true || m["link"] != "https://t.me/bruhanet_bot?start=ref_42" || m["invited"] != float64(5) || m["paid"] != float64(2) || m["rewardDays"] != float64(14) {
		t.Fatalf("referral = %s", raw)
	}
}

func TestMiniPromos(t *testing.T) {
	h, st, _ := newMiniPanel(t, &fakePanel{})
	st.userPromos = []model.UserPromo{
		{Code: "WELCOME", Kind: model.PromoDays, Value: 7, ActivatedAt: now},
		{Code: "SALE10", Kind: model.PromoPercent, Value: 10, ActivatedAt: now.Add(-time.Hour)},
	}
	_, m, raw := getMini(t, h, http.MethodGet, "/m/api/promos", "")
	items, _ := m["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["code"] != "WELCOME" || items[1].(map[string]any)["kind"] != "percent" {
		t.Fatalf("promos = %s", raw)
	}
}

func TestMiniSubscriptionLinkIsNotLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(24 * time.Hour))})
	w := miniReq(h, http.MethodPost, "/m/api/subscription-link", tma(t, miniToken, time.Minute, miniUser),
		map[string]string{"Content-Type": "application/json", "Origin": miniOrigin})
	if w.Code != 200 {
		t.Fatalf("code %d", w.Code)
	}
	if strings.Contains(buf.String(), "SECRET") || strings.Contains(buf.String(), "tma ") {
		t.Fatalf("log leaks secrets: %s", buf.String())
	}
}

func TestMiniInstructions(t *testing.T) {
	cfg := `{"platforms":{"ios":{"apps":[{"name":"Happ","blocks":[{"title":{"ru":"Установка"},"description":{"ru":"Поставь"},"svgIconKey":"DownloadIcon","svgIconColor":"cyan","buttons":[{"type":"subscriptionLink","link":"happ://add/{{SUBSCRIPTION_LINK}}","text":{"ru":"Добавить"}}]}]}]}}}`
	panel := &fakePanel{user: activeUser(now.Add(24 * time.Hour)), subpage: json.RawMessage(cfg)}
	h, _, _ := newMiniPanel(t, panel)
	for range 2 {
		code, m, raw := getMini(t, h, http.MethodGet, "/m/api/instructions", "")
		pl, _ := m["platforms"].([]any)
		if code != 200 || len(pl) != 1 || strings.Contains(raw, miniSubURL) {
			t.Fatalf("%d %s", code, raw)
		}
	}
	if panel.subpageCalls != 1 {
		t.Fatalf("panel called %d times, the second answer must come from the cache", panel.subpageCalls)
	}

	for name, p := range map[string]*fakePanel{
		"not in panel":   {err: remnawave.ErrUserNotFound},
		"config not set": {user: activeUser(now.Add(time.Hour)), subpageErr: remnawave.ErrSubpageConfigNotSet},
		"garbage config": {user: activeUser(now.Add(time.Hour)), subpage: json.RawMessage(`not json`)},
	} {
		h, _, _ := newMiniPanel(t, p)
		code, m, raw := getMini(t, h, http.MethodGet, "/m/api/instructions", "")
		if pl, ok := m["platforms"].([]any); code != 200 || !ok || len(pl) != 0 {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	h, _, _ = newMiniPanel(t, &fakePanel{user: activeUser(now.Add(time.Hour)), subpageErr: http.ErrHandlerTimeout})
	if code, m, _ := getMini(t, h, http.MethodGet, "/m/api/instructions", ""); code != 502 || m["error"] != "panel_unavailable" {
		t.Fatalf("panel down: %d %v", code, m)
	}
}
