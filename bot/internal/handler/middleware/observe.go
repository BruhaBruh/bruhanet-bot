package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

// Observe — span и метрики апдейта. Ставится между TelegramID и Logger, внутри Recovery;
// роут и panicked читаются после next.
func Observe(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		start := time.Now()
		ctx, span := observability.Tracer().Start(ctx, "update", trace.WithAttributes(
			attribute.String("update_type", tgupdate.Type(u)),
			attribute.String("telegram_id", strconv.FormatInt(tgupdate.UserID(u), 10)),
			attribute.String("request_id", RequestIDFrom(ctx)),
		))
		defer span.End()

		next(ctx, b, u)

		info := UpdateInfoFrom(ctx)
		route, result := info.Route(), "ok"
		span.SetName("update " + route)
		span.SetAttributes(attribute.String("route", route))
		if info.Panicked() {
			result = "panic"
			span.SetStatus(codes.Error, "panic")
		}
		observability.Updates.WithLabelValues(route, result).Inc()
		observability.UpdateDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	}
}
