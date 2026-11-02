// Package reminder напоминает юзерам об окончании подписки (за неделю и за сутки).
// Доставку делает outbox-воркер, пакет только ставит сообщения в очередь.
package reminder

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot/models"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
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

type Enqueuer interface {
	Enqueue(ctx context.Context, m model.OutboxMessage) error
}

// Render строит текст (MarkdownV2) и клавиатуру напоминания.
type Render func(kind Kind, expireAt time.Time, subURL string) (string, *models.InlineKeyboardMarkup)

type Users interface {
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

type Reminder struct {
	users  Users
	q      Enqueuer
	render Render
	rdb    *redis.Client
	now    func() time.Time
}

func New(users Users, q Enqueuer, render Render, rdb *redis.Client, now func() time.Time) *Reminder {
	if now == nil {
		now = time.Now
	}
	return &Reminder{users: users, q: q, render: render, rdb: rdb, now: now}
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
	text, kb := r.render(kind, u.ExpireAt, u.SubscriptionUrl)
	err = r.q.Enqueue(ctx, model.OutboxMessage{
		EventType: "reminder", ChatID: id, Text: text, ParseMode: string(models.ParseModeMarkdown), Keyboard: kb,
	})
	if err != nil {
		if delErr := r.rdb.Del(context.WithoutCancel(ctx), key).Err(); delErr != nil {
			stlog.Error(ctx, "failed to delete reminder key", "error", delErr)
		}
		stlog.Warn(ctx, "failed to enqueue reminder, will retry", "error", err)
		return
	}
	observability.RemindersSent.WithLabelValues(string(kind)).Inc()
	stlog.Info(ctx, "reminder sent")
}
