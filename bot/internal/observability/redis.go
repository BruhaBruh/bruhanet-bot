package observability

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
)

type redisHook struct{}

// RedisHook меряет команды Redis. В спан и метки идёт только имя команды, без ключей и значений.
func RedisHook() redis.Hook { return redisHook{} }

func (redisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}

func (redisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if isProbe(ctx) {
			return next(ctx, cmd)
		}
		op := strings.ToLower(cmd.Name())
		ctx, span := startSpan(ctx, "redis "+op, attribute.String("db.operation", op))
		start := time.Now()
		err := next(ctx, cmd)
		finish(span, "redis", op, start, ignoreNil(err))
		return err
	}
}

func (redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if isProbe(ctx) {
			return next(ctx, cmds)
		}
		ctx, span := startSpan(ctx, "redis pipeline", attribute.String("db.operation", "pipeline"))
		start := time.Now()
		err := next(ctx, cmds)
		finish(span, "redis", "pipeline", start, ignoreNil(err))
		return err
	}
}

func ignoreNil(err error) error {
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}
