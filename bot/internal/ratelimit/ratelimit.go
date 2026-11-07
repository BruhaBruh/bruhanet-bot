// Package ratelimit — счётчик неудач по ключу в окне (Redis): защита от перебора, например кодов подарка.
package ratelimit

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis считает неудачи в Redis: первая ставит срок жизни ключа, окно не продлевается дальше.
type Redis struct{ rdb *redis.Client }

func New(rdb *redis.Client) *Redis { return &Redis{rdb: rdb} }

// Fails — сколько неудач по ключу за текущее окно.
func (r *Redis) Fails(ctx context.Context, key string) (int, error) {
	n, err := r.rdb.Get(ctx, key).Int()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return n, err
}

// Fail записывает неудачу; ключ живёт window с первой неудачи.
func (r *Redis) Fail(ctx context.Context, key string, window time.Duration) error {
	n, err := r.rdb.Incr(ctx, key).Result()
	if err != nil {
		return err
	}
	if n == 1 {
		return r.rdb.Expire(ctx, key, window).Err()
	}
	return nil
}
