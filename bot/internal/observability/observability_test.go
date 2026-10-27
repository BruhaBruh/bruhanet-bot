package observability_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
)

func get(t *testing.T, r *observability.Readiness, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	observability.NewServer(":0", r).Handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return rec
}

func runOnce(t *testing.T, r *observability.Readiness) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go r.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, deps := r.Status(); len(deps) > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("readiness did not update")
}

func ok(context.Context) error { return nil }

func TestHealthz(t *testing.T) {
	rec := get(t, observability.NewReadiness(nil, time.Hour, time.Second), "/healthz")
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestReadyBeforeFirstCheck(t *testing.T) {
	r := observability.NewReadiness(map[string]observability.Check{"redis": ok}, time.Hour, time.Second)
	if rec := get(t, r, "/readyz"); rec.Code != 503 {
		t.Fatalf("code %d", rec.Code)
	}
	if v := testutil.ToFloat64(observability.Ready); v != 0 {
		t.Fatalf("ready=%v", v)
	}
}

func TestReadyAllOK(t *testing.T) {
	r := observability.NewReadiness(map[string]observability.Check{"redis": ok, "telegram": ok}, time.Hour, time.Second)
	runOnce(t, r)
	if rec := get(t, r, "/readyz"); rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	if v := testutil.ToFloat64(observability.DependencyUp.WithLabelValues("redis")); v != 1 {
		t.Fatalf("redis=%v", v)
	}
	if v := testutil.ToFloat64(observability.Ready); v != 1 {
		t.Fatalf("ready=%v", v)
	}
}

func TestReadyOneDown(t *testing.T) {
	r := observability.NewReadiness(map[string]observability.Check{
		"redis":     ok,
		"remnawave": func(context.Context) error { return errors.New("boom") },
	}, time.Hour, time.Second)
	runOnce(t, r)
	rec := get(t, r, "/readyz")
	if rec.Code != 503 {
		t.Fatalf("code %d", rec.Code)
	}
	var body struct {
		Ready        bool            `json:"ready"`
		Dependencies map[string]bool `json:"dependencies"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Ready || body.Dependencies["remnawave"] || !body.Dependencies["redis"] {
		t.Fatalf("body %+v", body)
	}
	if v := testutil.ToFloat64(observability.Ready); v != 0 {
		t.Fatalf("ready=%v", v)
	}
}

func TestReadyTimeout(t *testing.T) {
	r := observability.NewReadiness(map[string]observability.Check{
		"slow": func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}, time.Hour, 50*time.Millisecond)
	runOnce(t, r)
	if ready, deps := r.Status(); ready || deps["slow"] {
		t.Fatalf("ready=%v deps=%v", ready, deps)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	body := get(t, observability.NewReadiness(nil, time.Hour, time.Second), "/metrics").Body.String()
	for _, want := range []string{"bot_ready", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Fatalf("no %s in metrics", want)
		}
	}
}

func TestHealthcheckCommand(t *testing.T) {
	srv := httptest.NewServer(observability.NewServer(":0", observability.NewReadiness(nil, time.Hour, time.Second)).Handler)
	u, _ := url.Parse(srv.URL)
	if code := observability.Healthcheck(":" + u.Port()); code != 0 {
		t.Fatalf("code %d", code)
	}
	srv.Close()
	if code := observability.Healthcheck(":" + u.Port()); code != 1 {
		t.Fatalf("closed: code %d", code)
	}
}

func TestInitBusinessSeries(t *testing.T) {
	observability.InitBusinessSeries([]int{1, 6})
	if n := testutil.CollectAndCount(observability.Payments); n < 8 {
		t.Fatalf("payments series=%d", n)
	}
	if v := testutil.ToFloat64(observability.PaymentsConfirmedRubles.WithLabelValues("6")); v != 0 {
		t.Fatal(v)
	}
	if n := testutil.CollectAndCount(observability.RemindersSent); n < 2 {
		t.Fatalf("reminders series=%d", n)
	}
	if n := testutil.CollectAndCount(observability.BroadcastMessages); n < 2 {
		t.Fatalf("broadcast series=%d", n)
	}
}
