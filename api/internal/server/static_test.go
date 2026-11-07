package server_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/api/internal/server"
)

func TestSPAFallback(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	for _, p := range []string{"/", "/users/123", "/assets/missing.js"} {
		if w := do(h, http.MethodGet, p, "", ""); w.Code != http.StatusOK || w.Body.String() != indexHTML {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body)
		}
	}
	if w := do(h, http.MethodGet, "/assets/app.js", "", ""); w.Code != http.StatusOK || w.Body.String() != "console.log(1)" {
		t.Fatalf("asset: %d %s", w.Code, w.Body)
	}

	if w := do(h, http.MethodPost, "/users/123", "", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST static: %d", w.Code)
	}
	// метрика по шаблону маршрута, без id из пути
	if n := testutil.ToFloat64(server.Requests.WithLabelValues("/", "200")); n < 3 {
		t.Fatalf(`requests{route="/"} = %v`, n)
	}
	// шаблон вложенного api-мукса, а не "/api/"
	do(h, http.MethodGet, "/api/me", c, "")
	if n := testutil.ToFloat64(server.Requests.WithLabelValues("GET /api/me", "200")); n < 1 {
		t.Fatalf(`requests{route="GET /api/me"} = %v`, n)
	}

	for _, p := range []string{"/api/nope", "/api/"} {
		w := do(h, http.MethodGet, p, c, "")
		if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != `{"error":"not found"}` ||
			!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body)
		}
	}
}

// Заголовки безопасности стоят и на странице, и на api, и на отказе без сессии.
func TestSecurityHeadersThroughServer(t *testing.T) {
	h, _ := newServer(t, "http://localhost:5173")
	for _, p := range []string{"/", "/api/me", "/auth/bot"} {
		w := do(h, http.MethodGet, p, "", "")
		if w.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("%s: headers = %v", p, w.Header())
		}
	}
}
