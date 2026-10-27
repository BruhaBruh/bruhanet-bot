package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
)

func TestTracingNoopWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	start := time.Now()
	shutdown, err := observability.InitTracing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("слишком медленно")
	}
}
