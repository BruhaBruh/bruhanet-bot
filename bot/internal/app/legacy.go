package app

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// warnLegacyPayments предупреждает о платежах на проверке, оставшихся в Redis FSM со времён до Postgres.
// Ничего не меняет: такие платежи разбирает админ вручную.
func warnLegacyPayments(ctx context.Context, rdb *redis.Client) error {
	it := rdb.Scan(ctx, 0, "fsm:*", 100).Iterator()
	for it.Next(ctx) {
		st, err := rdb.HGet(ctx, it.Val(), "state").Result()
		if err != nil {
			continue // ключ исчез или не hash — не наш случай
		}
		if st == "await_photo" || st == "in_review" {
			stlog.Warn(ctx, "legacy fsm payment found, review it manually", "telegram_id", strings.TrimPrefix(it.Val(), "fsm:"))
		}
	}
	return it.Err()
}
