package remnawave_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

// 401/403 панели — ошибка, а не «юзер не найден» (readiness различает их).
func TestGetUserAuthErrorIsNotNotFound(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		rw, err := remnawave.New(srv.URL, "t", "c=1")
		if err != nil {
			t.Fatal(err)
		}
		_, err = rw.GetUser(context.Background(), 1)
		srv.Close()
		if err == nil || errors.Is(err, remnawave.ErrUserNotFound) {
			t.Fatalf("code %d: err=%v", code, err)
		}
	}
}

// ogen-клиент не должен плодить спаны в глобальный provider.
func TestClientEmitsNoSpans(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer srv.Close()
	rw, err := remnawave.New(srv.URL, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = rw.GetUser(context.Background(), 1)
	if n := len(sr.Ended()); n != 0 {
		t.Fatalf("spans=%d", n)
	}
}
