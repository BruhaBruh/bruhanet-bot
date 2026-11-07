// Package names — добор имён юзеров через getChat: те, кого знает только панель (синхронизация), приходят без имени,
// а после первого /start бот узнаёт его из апдейта сам (middleware.LastSeen).
package names

import (
	"context"
	"errors"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	batch   = 25
	gap     = 100 * time.Millisecond
	recheck = 7 * 24 * time.Hour
	timeout = 10 * time.Second
)

// Chat — то, что getChat знает о собеседнике.
type Chat struct{ Username, FirstName, LastName string }

// Store — core/store.
type Store interface {
	NamesDue(ctx context.Context, before time.Time, limit int) ([]int64, error)
	SaveChatName(ctx context.Context, id int64, username, first, last string, at time.Time) error
}

// Fetcher спрашивает Telegram о юзере.
type Fetcher func(ctx context.Context, id int64) (Chat, error)

// FromBot — Fetcher поверх getChat (клиент бота уже обёрнут метриками).
func FromBot(b *bot.Bot) Fetcher {
	return func(ctx context.Context, id int64) (Chat, error) {
		ch, err := b.GetChat(ctx, &bot.GetChatParams{ChatID: id})
		if err != nil {
			return Chat{}, err
		}
		return Chat{Username: ch.Username, FirstName: ch.FirstName, LastName: ch.LastName}, nil
	}
}

// Job добирает имена пачками.
type Job struct {
	st    Store
	fetch Fetcher
	now   func() time.Time
	gap   time.Duration
}

func New(st Store, fetch Fetcher, now func() time.Time) *Job {
	if now == nil {
		now = time.Now
	}
	return &Job{st: st, fetch: fetch, now: now, gap: gap}
}

// Run — один проход, до batch юзеров. Отказ Telegram (чата нет, бот заблокирован) тоже отмечается проверкой, чтобы
// не спрашивать снова раньше чем через неделю; 429 прерывает проход без отметки; сетевой сбой — без отметки, дальше.
func (j *Job) Run(ctx context.Context) {
	at := j.now()
	ids, err := j.st.NamesDue(ctx, at.Add(-recheck), batch)
	if err != nil {
		stlog.Error(ctx, "failed to list users without name", "error", err)
		return
	}
	filled := 0
	for i, id := range ids {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(j.gap):
			}
		}
		fctx, cancel := context.WithTimeout(ctx, timeout)
		ch, err := j.fetch(fctx, id)
		cancel()
		switch {
		case err == nil:
		case errors.Is(err, bot.ErrorForbidden), errors.Is(err, bot.ErrorBadRequest), errors.Is(err, bot.ErrorNotFound):
			ch = Chat{} // чата нет: просто отметка
		case bot.IsTooManyRequestsError(err):
			stlog.Warn(ctx, "name backfill stopped: rate limited", "error", err)
			return
		default:
			if ctx.Err() != nil {
				return
			}
			stlog.Error(ctx, "failed to get chat", "error", err)
			continue
		}
		if err = j.st.SaveChatName(ctx, id, ch.Username, ch.FirstName, ch.LastName, at); err != nil {
			stlog.Error(ctx, "failed to save chat name", "error", err)
			continue
		}
		if ch.FirstName != "" || ch.LastName != "" {
			filled++
		}
	}
	if len(ids) > 0 {
		stlog.Info(ctx, "name backfill done", "checked", len(ids), "filled", filled)
	}
}
