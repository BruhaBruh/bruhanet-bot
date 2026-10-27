package stlog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func logLine(t *testing.T, ctx context.Context, json_ bool) (string, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	opts := []stlog.Option{stlog.WithWriter(&buf), stlog.WithDefault(false), stlog.WithJSON(json_)}
	ctx = stlog.Wrap(ctx, stlog.New(opts...))
	stlog.Info(ctx, "hi")
	m := map[string]any{}
	if json_ {
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
	}
	return buf.String(), m
}

func spanCtx() (context.Context, trace.SpanContext) {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc), sc
}

func TestLogHasTraceID(t *testing.T) {
	ctx, sc := spanCtx()
	_, m := logLine(t, ctx, true)
	if m["trace_id"] != sc.TraceID().String() || m["span_id"] != sc.SpanID().String() {
		t.Fatalf("line = %v", m)
	}
	text, _ := logLine(t, ctx, false)
	if !bytes.Contains([]byte(text), []byte("trace_id="+sc.TraceID().String())) {
		t.Fatalf("text = %s", text)
	}
}

func TestLogNoTraceIDWithoutSpan(t *testing.T) {
	_, m := logLine(t, context.Background(), true)
	if _, ok := m["trace_id"]; ok {
		t.Fatalf("line = %v", m)
	}
	_ = slog.LevelInfo
}
