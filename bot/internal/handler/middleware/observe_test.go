package middleware_test

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func recorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func attrs(kv []attribute.KeyValue) map[string]string {
	m := map[string]string{}
	for _, a := range kv {
		m[string(a.Key)] = a.Value.String()
	}
	return m
}

func TestObserveRecordsUpdate(t *testing.T) {
	rec := recorder(t)
	before := testutil.ToFloat64(observability.Updates.WithLabelValues("callback:menu", "ok"))
	chain(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		middleware.UpdateInfoFrom(ctx).SetRoute("callback:menu")
	}, middleware.RequestID, middleware.TelegramID, middleware.Observe, middleware.Recovery)(
		context.Background(), nil, tgtest.Callback(42, "menu"))

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Name() != "update callback:menu" {
		t.Fatalf("spans = %v", spans)
	}
	a := attrs(spans[0].Attributes())
	if a["route"] != "callback:menu" || a["request_id"] == "" || a["telegram_id"] != "42" || a["update_type"] == "" {
		t.Fatalf("attrs = %v", a)
	}
	if got := testutil.ToFloat64(observability.Updates.WithLabelValues("callback:menu", "ok")); got != before+1 {
		t.Fatalf("Updates = %v, want %v", got, before+1)
	}
}

func TestObservePanic(t *testing.T) {
	rec := recorder(t)
	before := testutil.ToFloat64(observability.Updates.WithLabelValues("boom", "panic"))
	chain(func(ctx context.Context, b *bot.Bot, u *models.Update) {
		middleware.UpdateInfoFrom(ctx).SetRoute("boom")
		boom(ctx, b, u)
	}, middleware.RequestID, middleware.TelegramID, middleware.Observe, middleware.Recovery)(
		context.Background(), nil, tgtest.Message(42, "x"))

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("spans = %v", spans)
	}
	if got := testutil.ToFloat64(observability.Updates.WithLabelValues("boom", "panic")); got != before+1 {
		t.Fatalf("Updates = %v, want %v", got, before+1)
	}
}
