package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/core/model"
)

func TestPaymentsBadStatus(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	for _, q := range []string{"status=nope", "from=2026-13-01", "to=yesterday", "page=0", "page=x", "from=2026-11-05&to=2026-11-01"} {
		w := do(h, http.MethodGet, "/api/payments?"+q, c, "")
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"bad_request"}` {
			t.Errorf("%s: %d %s", q, w.Code, w.Body)
		}
	}
}

// now = 2026-11-02 12:00 UTC = 15:00 МСК; последние 30 дней — с 04.10 по 02.11 включительно.
func TestPaymentsDefaults(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.payments = []model.Payment{{ID: 1, TelegramID: 7, Month: 1, Price: 150, Status: model.PaymentPending, CreatedAt: now}}
	st.revenue = 150

	w := do(h, http.MethodGet, "/api/payments", c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	msk := time.FixedZone("MSK", 3*3600)
	from := time.Date(2026, 10, 4, 0, 0, 0, 0, msk)
	to := time.Date(2026, 11, 3, 0, 0, 0, 0, msk)
	if !st.lastF.From.Equal(from) || !st.lastF.To.Equal(to) || st.lastF.Limit != 50 || st.lastF.Offset != 0 || st.lastF.Status != "" {
		t.Fatalf("filter = %+v", st.lastF)
	}
	if !st.lastRev[0].Equal(from) || !st.lastRev[1].Equal(to) {
		t.Fatalf("revenue period = %v", st.lastRev)
	}
	var out struct {
		Items   []model.Payment
		Total   int
		Revenue int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Items) != 1 || out.Total != 1 || out.Revenue != 150 {
		t.Fatalf("%s %v", w.Body, err)
	}

	do(h, http.MethodGet, "/api/payments?status=confirmed&from=2026-10-01&to=2026-10-31&page=3", c, "")
	if st.lastF.Status != model.PaymentConfirmed || st.lastF.Offset != 100 ||
		!st.lastF.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, msk)) || !st.lastF.To.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, msk)) {
		t.Fatalf("filter = %+v", st.lastF)
	}
}

func TestPaymentPhoto(t *testing.T) {
	jpeg := "\xff\xd8\xff\xe0" + strings.Repeat("x", 600)
	h, st, tg := newServerTG(t, "http://localhost:5173", &fakePanel{}, 0)
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.payments = []model.Payment{
		{ID: 1, TelegramID: 7, PhotoFileID: "file-1"},
		{ID: 2, TelegramID: 7},
	}
	tg.file = jpeg

	// без сессии — 401, до Telegram дело не доходит
	if w := do(h, http.MethodGet, "/api/payments/1/photo", "", ""); w.Code != http.StatusUnauthorized || tg.opened != "" {
		t.Fatalf("no session: %d, opened %q", w.Code, tg.opened)
	}
	w := do(h, http.MethodGet, "/api/payments/1/photo", c, "")
	if w.Code != http.StatusOK || w.Body.String() != jpeg || tg.opened != "file-1" {
		t.Fatalf("%d, opened %q, len %d", w.Code, tg.opened, w.Body.Len())
	}
	if ct, cc := w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"); ct != "image/jpeg" || !strings.HasPrefix(cc, "private") {
		t.Fatalf("content-type %q, cache-control %q", ct, cc)
	}
	if nosniff := w.Header().Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Fatalf("nosniff = %q", nosniff)
	}

	for name, path := range map[string]string{"no photo": "/api/payments/2/photo", "no payment": "/api/payments/9/photo"} {
		if w := do(h, http.MethodGet, path, c, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := do(h, http.MethodGet, "/api/payments/x/photo", c, ""); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", w.Code)
	}

	// не картинка не уходит браузеру, что бы ни прислали
	tg.file = "<script>alert(1)</script>"
	if w := do(h, http.MethodGet, "/api/payments/1/photo", c, ""); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "script") {
		t.Fatalf("html: %d %s", w.Code, w.Body)
	}

	tg.openErr = errors.Join(bot.ErrorBadRequest, errors.New("wrong file_id"))
	if w := do(h, http.MethodGet, "/api/payments/1/photo", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("stale file: %d", w.Code)
	}
	tg.openErr = errors.New("connection refused")
	if w := do(h, http.MethodGet, "/api/payments/1/photo", c, ""); w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "refused") {
		t.Fatalf("telegram down: %d %s", w.Code, w.Body)
	}
}

// file_id клиенту не отдаётся: только признак.
func TestPaymentsHideFileID(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.payments = []model.Payment{{ID: 1, TelegramID: 7, PhotoFileID: "secret-file", HasPhoto: true, CreatedAt: now}}
	w := do(h, http.MethodGet, "/api/payments", c, "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "secret-file") || !strings.Contains(w.Body.String(), `"hasPhoto":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
