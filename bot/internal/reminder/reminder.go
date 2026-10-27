// Package reminder напоминает юзерам об окончании подписки (за неделю и за сутки).
// Доставку делает Notifier, пакет про Telegram не знает.
package reminder

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

type Kind string

const (
	Week Kind = "week"
	Day  Kind = "day"
)

const (
	dedupTTL   = 8 * 24 * time.Hour
	dayWindow  = 24 * time.Hour
	weekWindow = 7 * 24 * time.Hour
)

// ErrUndeliverable — получатель недоступен, повтор бессмыслен (Notifier оборачивает им такие ошибки).
var ErrUndeliverable = errors.New("reminder: recipient unreachable")

type Notifier interface {
	Remind(ctx context.Context, telegramID int64, kind Kind, expireAt time.Time, subURL string) error
}

type Users interface {
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

type Reminder struct {
	users Users
	n     Notifier
	rdb   *redis.Client
	now   func() time.Time
}

func New(users Users, n Notifier, rdb *redis.Client, now func() time.Time) *Reminder {
	if now == nil {
		now = time.Now
	}
	return &Reminder{users: users, n: n, rdb: rdb, now: now}
}

// Run — один проход по всем юзерам панели.
func (r *Reminder) Run(ctx context.Context) {
	now := r.now()
	for u, err := range r.users.Users(ctx) {
		if err != nil {
			stlog.Error(ctx, "failed to list remnawave users", "error", err)
			return
		}
		id, ok := remnawave.TelegramID(u)
		if !ok {
			continue
		}
		left := u.ExpireAt.Sub(now)
		var kind Kind
		switch {
		case left <= 0:
			continue
		case left <= dayWindow:
			kind = Day
		case left <= weekWindow:
			kind = Week
		default:
			continue
		}
		r.remind(stlog.With(ctx, "telegram_id", id, "kind", string(kind)), u, id, kind)
	}
}

func (r *Reminder) remind(ctx context.Context, u api.UserItemInfo, id int64, kind Kind) {
	key := fmt.Sprintf("notify:%d:%d:%s", id, u.ExpireAt.Unix(), kind)
	fresh, err := r.rdb.SetNX(ctx, key, 1, dedupTTL).Result()
	if err != nil {
		stlog.Error(ctx, "failed to set reminder key", "error", err)
		return
	}
	if !fresh {
		return
	}
	err = r.n.Remind(ctx, id, kind, u.ExpireAt, u.SubscriptionUrl)
	switch {
	case err == nil:
		stlog.Info(ctx, "reminder sent")
	case errors.Is(err, ErrUndeliverable):
		stlog.Warn(ctx, "failed to send reminder, not retrying", "error", err)
	default:
		if delErr := r.rdb.Del(context.WithoutCancel(ctx), key).Err(); delErr != nil {
			stlog.Error(ctx, "failed to delete reminder key", "error", delErr)
		}
		stlog.Warn(ctx, "failed to send reminder, will retry", "error", err)
	}
}
