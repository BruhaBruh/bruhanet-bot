package remnawave_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
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

// SetDeviceLimit шлёт в панель только id и hwidDeviceLimit: срок и статус не трогаются.
func TestSetDeviceLimitSendsOnlyLimit(t *testing.T) {
	var patched map[string]any
	user := func(limit api.NilInt) api.UserItemInfo {
		return api.UserItemInfo{
			ID: 42, Status: api.UserItemInfoStatusACTIVE, TrafficLimitStrategy: api.UserItemInfoTrafficLimitStrategyNORESET,
			Email: api.NewNilString("a@b.co"), HwidDeviceLimit: limit, ActiveInternalSquads: []api.ActiveInternalSquad{},
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(api.GetUsersStreamResponse{Response: api.GetUsersStreamResponseResponse{
				Users: []api.UserItemInfo{user(api.NilInt{Null: true})},
			}})
		default:
			_ = json.NewDecoder(r.Body).Decode(&patched)
			_ = json.NewEncoder(w).Encode(api.UserResponse{Response: user(api.NewNilInt(7))})
		}
	}))
	defer srv.Close()
	rw, err := remnawave.New(srv.URL, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	u, err := rw.SetDeviceLimit(context.Background(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if limit, ok := remnawave.DeviceLimit(*u); !ok || limit != 7 {
		t.Fatalf("limit = %d %v", limit, ok)
	}
	if len(patched) != 2 || patched["hwidDeviceLimit"] != float64(7) || patched["id"] != float64(42) {
		t.Fatalf("body = %v", patched)
	}
}
