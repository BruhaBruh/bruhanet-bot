package observability

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/bruhabruh/bruhanet/bot"

// InitTracing включает экспорт трейсов по OTLP/HTTP (порт 4318), если задан OTEL_EXPORTER_OTLP_ENDPOINT;
// иначе остаётся глобальный no-op provider. Эту же переменную читает и сам SDK (otlptracehttp.New).
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	// Ошибки экспорта (Tempo лежит) — WARN, чтобы Loki-алерт на ERROR не срабатывал.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { slog.Warn("otel error", "error", err) }))
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewSchemaless(semconv.ServiceName("bruhanet-bot"))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func Tracer() trace.Tracer { return otel.Tracer(tracerName) }

type requestIDKey struct{}

// WithRequestID кладёт id апдейта в ctx; дочерние спаны внешних вызовов подхватывают его атрибутом.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

type probeKey struct{}

// WithProbe помечает ctx проверки зависимости (readiness): инструменты внешних вызовов её не учитывают.
func WithProbe(ctx context.Context) context.Context { return context.WithValue(ctx, probeKey{}, true) }

func isProbe(ctx context.Context) bool { v, _ := ctx.Value(probeKey{}).(bool); return v }

// startSpan открывает дочерний спан; request_id добавляется, если он есть в ctx.
func startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if id := RequestID(ctx); id != "" {
		attrs = append(attrs, attribute.String("request_id", id))
	}
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// finish закрывает спан и пишет метрики внешнего вызова (request_id в метки не попадает).
func finish(span trace.Span, service, op string, start time.Time, err error, observeDuration ...bool) {
	result := "ok"
	if err != nil {
		result = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
	ExternalRequests.WithLabelValues(service, op, result).Inc()
	if len(observeDuration) == 0 || observeDuration[0] {
		ExternalDuration.WithLabelValues(service, op).Observe(time.Since(start).Seconds())
	}
}
