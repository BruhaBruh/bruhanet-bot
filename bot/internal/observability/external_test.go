package observability_test

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func recorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func metricsText(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(observability.NewServer("", nil).Handler)
	defer srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/metrics", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func tgCall(t *testing.T, status int, path string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	defer srv.Close()
	c := &http.Client{Transport: observability.TelegramTransport(http.DefaultTransport)}
	req, _ := http.NewRequestWithContext(observability.WithRequestID(context.Background(), "r1"), http.MethodPost, srv.URL+path, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func TestTelegramTransportHidesToken(t *testing.T) {
	rec := recorder(t)
	c := observability.ExternalRequests.WithLabelValues("telegram", "sendMessage", "ok")
	before := testutil.ToFloat64(c)
	tgCall(t, 200, "/bot123:SECRET/sendMessage")

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "telegram sendMessage" {
		t.Fatalf("spans = %v", spans)
	}
	got := map[string]string{}
	for _, a := range spans[0].Attributes() {
		got[string(a.Key)] = a.Value.String()
		if strings.Contains(a.Value.String(), "SECRET") {
			t.Fatalf("token leaked: %v", a)
		}
	}
	if got["telegram.method"] != "sendMessage" || got["http.status_code"] != "200" || got["request_id"] != "r1" {
		t.Fatalf("attrs = %v", got)
	}
	if testutil.ToFloat64(c) != before+1 {
		t.Fatal("metric not incremented")
	}
	if strings.Contains(metricsText(t), "SECRET") {
		t.Fatal("token in /metrics")
	}
}

func TestTelegramTransportSkipsGetUpdates(t *testing.T) {
	rec := recorder(t)
	c := observability.ExternalRequests.WithLabelValues("telegram", "getUpdates", "ok")
	before := testutil.ToFloat64(c)
	tgCall(t, 200, "/bot123:SECRET/getUpdates")
	if len(rec.Ended()) != 0 {
		t.Fatalf("spans = %v", rec.Ended())
	}
	if testutil.ToFloat64(c) != before+1 {
		t.Fatal("metric not incremented")
	}
}

func TestTelegramTransportError(t *testing.T) {
	c := observability.ExternalRequests.WithLabelValues("telegram", "sendPhoto", "error")
	before := testutil.ToFloat64(c)
	rec := recorder(t)
	tgCall(t, 500, "/bot1:X/sendPhoto")
	if sp := rec.Ended(); len(sp) != 1 || sp[0].Status().Code != codes.Error {
		t.Fatalf("spans = %v", sp)
	}
	if testutil.ToFloat64(c) != before+1 {
		t.Fatal("error metric not incremented")
	}
}

type fakeRW struct{ err error }

func (f fakeRW) GetUser(context.Context, int64) (*api.UserItemInfo, error) { return nil, f.err }
func (f fakeRW) CreateUser(context.Context, int64, string, uuid.UUID) (*api.UserItemInfo, error) {
	return nil, f.err
}

func (f fakeRW) UpdateUser(context.Context, int64, string, time.Time, uuid.UUID) (*api.UserItemInfo, error) {
	return nil, f.err
}

func (f fakeRW) Users(context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		yield(api.UserItemInfo{}, nil)
		yield(api.UserItemInfo{}, nil)
	}
}

func TestRemnawaveInstrumentNotFoundIsOK(t *testing.T) {
	rec := recorder(t)
	ok := observability.ExternalRequests.WithLabelValues("remnawave", "GetUser", "ok")
	bad := observability.ExternalRequests.WithLabelValues("remnawave", "GetUser", "error")
	okBefore, badBefore := testutil.ToFloat64(ok), testutil.ToFloat64(bad)

	ctx := observability.WithRequestID(context.Background(), "r1")
	_, err := observability.InstrumentRemnawave(fakeRW{remnawave.ErrUserNotFound}).GetUser(ctx, 1)
	if !errors.Is(err, remnawave.ErrUserNotFound) {
		t.Fatalf("err = %v", err)
	}
	s := rec.Ended()[0]
	if s.Name() != "remnawave GetUser" || s.Status().Code == codes.Error || testutil.ToFloat64(ok) != okBefore+1 {
		t.Fatalf("not-found must be ok: %v", s.Status())
	}
	var rid string
	for _, a := range s.Attributes() {
		if a.Key == "request_id" {
			rid = a.Value.String()
		}
	}
	if rid != "r1" {
		t.Fatalf("request_id = %q", rid)
	}

	_, _ = observability.InstrumentRemnawave(fakeRW{errors.New("boom")}).GetUser(ctx, 1)
	if testutil.ToFloat64(bad) != badBefore+1 {
		t.Fatal("other error must count as error")
	}
}

func TestRemnawaveUsersOneSpan(t *testing.T) {
	rec := recorder(t)
	for range observability.InstrumentRemnawave(fakeRW{}).Users(context.Background()) {
	}
	if n := len(rec.Ended()); n != 1 {
		t.Fatalf("spans = %d", n)
	}
}

func TestRedisHookNoArgs(t *testing.T) {
	rec := recorder(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rdb.AddHook(observability.RedisHook())
	ctx := context.Background()

	if err := rdb.Set(ctx, "fsm:42", `{"secret":"payload"}`, 0).Err(); err != nil {
		t.Fatal(err)
	}
	okGet := observability.ExternalRequests.WithLabelValues("redis", "get", "ok")
	before := testutil.ToFloat64(okGet)
	if err := rdb.Get(ctx, "missing").Err(); !errors.Is(err, redis.Nil) {
		t.Fatalf("err = %v", err)
	}
	if testutil.ToFloat64(okGet) != before+1 {
		t.Fatal("redis.Nil must be ok")
	}

	spans := rec.Ended()
	hasSet := false
	for _, s := range spans {
		hasSet = hasSet || s.Name() == "redis set"
	}
	if !hasSet {
		t.Fatalf("no redis set span: %v", spans)
	}
	for _, s := range spans {
		for _, a := range s.Attributes() {
			if v := a.Value.String(); strings.Contains(v, "fsm:42") || strings.Contains(v, "payload") {
				t.Fatalf("args leaked: %v", a)
			}
		}
	}
	if m := metricsText(t); strings.Contains(m, "fsm:42") || strings.Contains(m, "payload") {
		t.Fatal("args in /metrics")
	}
}

func histCount(t *testing.T, op string) uint64 {
	t.Helper()
	m := &dto.Metric{}
	if err := observability.ExternalDuration.WithLabelValues("telegram", op).(prometheus.Metric).Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

func TestGetUpdatesSkipsDuration(t *testing.T) {
	recorder(t)
	before := histCount(t, "getUpdates")
	tgCall(t, 200, "/bot1:X/getUpdates")
	if histCount(t, "getUpdates") != before {
		t.Fatal("getUpdates must not observe duration")
	}
}

func TestProbeIsNotInstrumented(t *testing.T) {
	rec := recorder(t)
	ctx := observability.WithProbe(context.Background())

	tg := observability.ExternalRequests.WithLabelValues("telegram", "getMe", "ok")
	tgBefore := testutil.ToFloat64(tg)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/bot1:X/getMe", nil)
	resp, err := (&http.Client{Transport: observability.TelegramTransport(http.DefaultTransport)}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	rdb.AddHook(observability.RedisHook())
	ping := observability.ExternalRequests.WithLabelValues("redis", "ping", "ok")
	pingBefore := testutil.ToFloat64(ping)
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	if testutil.ToFloat64(tg) != tgBefore || testutil.ToFloat64(ping) != pingBefore {
		t.Fatal("probe counted in metrics")
	}
	for _, s := range rec.Ended() {
		if s.Name() == "redis ping" || s.Name() == "telegram getMe" {
			t.Fatalf("probe span: %s", s.Name())
		}
	}
}
