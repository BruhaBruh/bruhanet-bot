package server_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func promoRequest(t *testing.T, h http.Handler, code string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/m/api/promos/activate",
		strings.NewReader(`{"code":"`+code+`"}`))
	r.Header.Set("Authorization", tma(t, miniToken, time.Minute, miniUser))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", miniOrigin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func seedPromos(st *fakeStore) {
	st.promos = []model.PromoWithUsage{
		{Promo: model.Promo{ID: 1, Code: "WELCOME", Kind: model.PromoDays, Value: 7}},
		{Promo: model.Promo{ID: 2, Code: "SALE10", Kind: model.PromoPercent, Value: 10}},
	}
}

func TestMiniPromoActivate(t *testing.T) {
	panel := &fakePanel{user: activeUser(now.Add(24 * time.Hour))}
	h, st, _ := newMiniPanel(t, panel)
	seedPromos(st)
	w := promoRequest(t, h, " welcome ")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"addedDays":7`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(panel.updates) != 1 || !panel.updates[0].Equal(now.Add(8*24*time.Hour)) {
		t.Fatalf("panel updates = %v", panel.updates)
	}
	if w = promoRequest(t, h, "WELCOME"); w.Code != 422 {
		t.Fatalf("second activation must be refused, got %d", w.Code)
	}
}

func TestMiniPromoRefusalsLookTheSame(t *testing.T) {
	panel := &fakePanel{user: activeUser(now.Add(24 * time.Hour))}
	h, st, _ := newMiniPanel(t, panel)
	seedPromos(st)
	unknown := promoRequest(t, h, "NOPE")
	if unknown.Code != 422 || !strings.Contains(unknown.Body.String(), "promo_unavailable") {
		t.Fatalf("unknown: %d %s", unknown.Code, unknown.Body.String())
	}
	if w := promoRequest(t, h, "SALE10"); w.Code != 422 || w.Body.String() != unknown.Body.String() {
		t.Fatalf("discount code must look like an unknown one: %d %s", w.Code, w.Body.String())
	}
	blockedAt := now
	st.users[0].BlockedAt = &blockedAt
	if w := promoRequest(t, h, "WELCOME"); w.Code != 422 || w.Body.String() != unknown.Body.String() {
		t.Fatalf("blocked user must get the neutral refusal: %d %s", w.Code, w.Body.String())
	}
	if len(panel.updates) != 0 {
		t.Fatalf("refusals must not touch the panel: %v", panel.updates)
	}
}

func TestMiniPromoNoSubscriptionAndPanelDown(t *testing.T) {
	h, st, _ := newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	seedPromos(st)
	if w := promoRequest(t, h, "WELCOME"); w.Code != 409 || !strings.Contains(w.Body.String(), "no_subscription") {
		t.Fatalf("no panel user: %d %s", w.Code, w.Body.String())
	}
	panel := &fakePanel{user: activeUser(now.Add(time.Hour)), updateErr: http.ErrHandlerTimeout}
	h, st, _ = newMiniPanel(t, panel)
	seedPromos(st)
	if w := promoRequest(t, h, "WELCOME"); w.Code != 502 || !strings.Contains(w.Body.String(), "panel_unavailable") {
		t.Fatalf("panel down: %d %s", w.Code, w.Body.String())
	}
	panel.updateErr = nil
	if w := promoRequest(t, h, "WELCOME"); w.Code != 200 {
		t.Fatalf("a panel failure must not burn the code, retry got %d", w.Code)
	}
}

func TestMiniPromoRateLimit(t *testing.T) {
	h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(time.Hour))})
	for i := range 5 {
		if w := promoRequest(t, h, "NOPE"); w.Code != 422 {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	if w := promoRequest(t, h, "NOPE"); w.Code != 429 {
		t.Fatalf("6th attempt in a minute must be limited, got %d", w.Code)
	}
}

func TestMiniPromoDoubleSubmit(t *testing.T) {
	h, st, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(24 * time.Hour))})
	seedPromos(st)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- promoRequest(t, h, "WELCOME").Code
		}()
	}
	wg.Wait()
	close(codes)
	ok, refused := 0, 0
	for c := range codes {
		switch c {
		case 200:
			ok++
		case 422:
			refused++
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("ok=%d refused=%d", ok, refused)
	}
}

func TestMiniPromoBodyLimit(t *testing.T) {
	h, _, _ := newMiniPanel(t, &fakePanel{user: activeUser(now.Add(time.Hour))})
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/m/api/promos/activate",
		bytes.NewReader(append([]byte(`{"code":"`), bytes.Repeat([]byte("A"), 5000)...)))
	r.Header.Set("Authorization", tma(t, miniToken, time.Minute, miniUser))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", miniOrigin)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d", w.Code)
	}
}

// Любой сбой панели при чтении юзера в активации — 502 с русским текстом, а не 500.
func TestMiniPromoPanelReadFailureIs502(t *testing.T) {
	h, st, _ := newMiniPanel(t, &fakePanel{err: http.ErrHandlerTimeout})
	seedPromos(st)
	w := promoRequest(t, h, "WELCOME")
	if w.Code != 502 || !strings.Contains(w.Body.String(), "panel_unavailable") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
