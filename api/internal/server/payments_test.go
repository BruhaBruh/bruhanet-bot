package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

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
