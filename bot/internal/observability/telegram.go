package observability

import (
	"context"
	"errors"
	"net/http"
	"path"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var errTelegramStatus = errors.New("telegram: http status >= 400")

type telegramTransport struct{ next http.RoundTripper }

// TelegramTransport меряет вызовы Bot API. Операция — последний сегмент пути (метод),
// токен из /bot<TOKEN>/... никуда не попадает. getUpdates (long poll) — только метрика, без спана.
func TelegramTransport(next http.RoundTripper) http.RoundTripper { return telegramTransport{next} }

func (t telegramTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if isProbe(req.Context()) {
		return t.next.RoundTrip(req)
	}
	op := path.Base(req.URL.Path)
	start := time.Now()
	span := trace.Span(noop.Span{})
	if op != "getUpdates" {
		var ctx context.Context
		ctx, span = startSpan(req.Context(), "telegram "+op, attribute.String("telegram.method", op))
		req = req.WithContext(ctx)
	}
	resp, err := t.next.RoundTrip(req)
	failed := err
	if err == nil {
		span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		if resp.StatusCode >= http.StatusBadRequest {
			failed = errTelegramStatus
		}
	}
	if op == "getUpdates" && errors.Is(err, context.Canceled) {
		failed = nil // остановка при shutdown — не ошибка
	}
	finish(span, "telegram", op, start, failed, op != "getUpdates") // long poll ~59с портит гистограмму
	return resp, err
}
