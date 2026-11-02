package observability

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type pgCall struct {
	span  trace.Span
	op    string
	start time.Time
}

type pgCallKey struct{}

type postgresTracer struct{}

// PostgresTracer меряет запросы pgx. В спан и метки идёт только первое слово SQL, без текста и аргументов.
func PostgresTracer() pgx.QueryTracer { return postgresTracer{} }

func (postgresTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if isProbe(ctx) {
		return ctx
	}
	op := "query"
	if f := strings.Fields(data.SQL); len(f) > 0 {
		op = strings.ToLower(f[0])
	}
	ctx, span := startSpan(ctx, "postgres "+op, attribute.String("db.operation", op))
	return context.WithValue(ctx, pgCallKey{}, &pgCall{span: span, op: op, start: time.Now()})
}

func (postgresTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if c, ok := ctx.Value(pgCallKey{}).(*pgCall); ok {
		finish(c.span, "postgres", c.op, c.start, data.Err)
	}
}
