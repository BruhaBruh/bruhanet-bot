package reminder_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-telegram/bot/models"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/reminder"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

var now = time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC)

const userID int64 = 42

type call struct {
	id   int64
	kind reminder.Kind
	exp  time.Time
	sub  string
}

type fakeEnq struct {
	mu    sync.Mutex
	calls []call
	msgs  []model.OutboxMessage
	errs  map[int64]error
}

func render(_ context.Context, kind reminder.Kind, exp time.Time, sub string) (string, *models.InlineKeyboardMarkup) {
	return fmt.Sprintf("%s|%d|%s", kind, exp.Unix(), sub), nil
}

func (f *fakeEnq) Enqueue(_ context.Context, m model.OutboxMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, f.parse(m))
	if err := f.errs[m.ChatID]; err != nil {
		return err
	}
	f.msgs = append(f.msgs, m)
	return nil
}

func (f *fakeEnq) parse(m model.OutboxMessage) call {
	parts := strings.SplitN(m.Text, "|", 3)
	unix, _ := strconv.ParseInt(parts[1], 10, 64)
	return call{m.ChatID, reminder.Kind(parts[0]), time.Unix(unix, 0), parts[2]}
}

func (f *fakeEnq) forID(id int64) []call {
	var out []call
	for _, c := range f.calls {
		if c.id == id {
			out = append(out, c)
		}
	}
	return out
}

type env struct {
	rw *tgtest.Remnawave
	n  *fakeEnq
	mr *miniredis.Miniredis
	r  *reminder.Reminder
	db *redis.Client
}

func newEnv(t *testing.T) *env {
	t.Helper()
	mr := miniredis.RunT(t)
	db := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = db.Close() })
	e := &env{
		rw: tgtest.NewRemnawave(func() time.Time { return now }),
		n:  &fakeEnq{errs: map[int64]error{}},
		mr: mr,
		db: db,
	}
	e.r = e.at(now)
	return e
}

func (e *env) at(t time.Time) *reminder.Reminder {
	return reminder.New(e.rw, e.n, render, e.db, func() time.Time { return t })
}

func TestWindows(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(1001, now.Add(8*24*time.Hour))
	e.rw.AddUser(1002, now.Add(3*24*time.Hour))
	e.rw.AddUser(1003, now.Add(12*time.Hour))
	e.rw.AddUser(1004, now.Add(-time.Hour))
	e.r.Run(t.Context())

	if len(e.n.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(e.n.calls))
	}
	week, day := e.n.forID(1002), e.n.forID(1003)
	if len(week) != 1 || week[0].kind != reminder.Week || len(day) != 1 || day[0].kind != reminder.Day {
		t.Fatalf("week=%v day=%v", week, day)
	}
	if week[0].sub == "" || !week[0].exp.Equal(now.Add(3*24*time.Hour)) {
		t.Fatalf("call = %+v", week[0])
	}
}

func TestReminderDedup(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(3*24*time.Hour))
	e.r.Run(t.Context())
	e.r.Run(t.Context())
	if len(e.n.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(e.n.calls))
	}
}

func TestAfterRenewal(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(3*24*time.Hour))
	e.r.Run(t.Context())
	e.rw.AddUser(userID, now.Add(5*24*time.Hour))
	e.r.Run(t.Context())
	if len(e.n.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(e.n.calls))
	}
}

func TestWeekThenDay(t *testing.T) {
	e := newEnv(t)
	exp := now.Add(3 * 24 * time.Hour)
	e.rw.AddUser(userID, exp)
	e.r.Run(t.Context())

	later := e.at(exp.Add(-12 * time.Hour))
	later.Run(t.Context())
	if len(e.n.calls) != 2 || e.n.calls[1].kind != reminder.Day {
		t.Fatalf("calls = %+v", e.n.calls)
	}
	later.Run(t.Context())
	if len(e.n.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(e.n.calls))
	}
}

func TestTemporaryFailureRetries(t *testing.T) {
	e := newEnv(t)
	exp := now.Add(3 * 24 * time.Hour)
	e.rw.AddUser(userID, exp)
	key := fmt.Sprintf("notify:%d:%d:week", userID, exp.Unix())

	e.n.errs[userID] = errors.New("oops")
	e.r.Run(t.Context())
	if e.mr.Exists(key) {
		t.Fatal("ключ не должен остаться после временной ошибки")
	}
	delete(e.n.errs, userID)
	e.r.Run(t.Context())
	if !e.mr.Exists(key) {
		t.Fatal("ключ должен быть после успешной отправки")
	}
	if len(e.n.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(e.n.calls))
	}
}

func TestReminderMessageFields(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(3*24*time.Hour))
	e.r.Run(t.Context())
	if len(e.n.msgs) != 1 {
		t.Fatalf("msgs = %d", len(e.n.msgs))
	}
	m := e.n.msgs[0]
	if m.EventType != "reminder" || m.ChatID != userID || m.ParseMode != string(models.ParseModeHTML) {
		t.Fatalf("msg = %+v", m)
	}
}

func TestSkipsWithoutTelegramID(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUserWithoutTelegramID(7, now.Add(3*24*time.Hour))
	e.r.Run(t.Context())
	if len(e.n.calls) != 0 {
		t.Fatalf("calls = %d, want 0", len(e.n.calls))
	}
}

func TestStopsOnPanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(1001, now.Add(3*24*time.Hour))
	e.rw.AddUser(1002, now.Add(3*24*time.Hour))
	e.rw.FailUsersAfter(1, errors.New("panel down"))
	ctx, buf := tgtest.LogContext()
	e.r.Run(ctx)
	if len(e.n.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(e.n.calls))
	}
	tgtest.FindLog(t, buf, "failed to list remnawave users")
}

func TestReminderLogsSent(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(1002, now.Add(3*24*time.Hour))
	l, buf := tgtest.Logger()
	e.r.Run(stlog.Wrap(t.Context(), l))

	got := tgtest.FindLog(t, buf, "reminder queued")
	if got["telegram_id"] != float64(1002) || got["kind"] != "week" {
		t.Fatalf("entry = %v", got)
	}
}

func TestReminderMetric(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(3*24*time.Hour))
	c := observability.RemindersSent.WithLabelValues("week")
	before := testutil.ToFloat64(c)

	e.r.Run(t.Context())

	if got := testutil.ToFloat64(c) - before; got != 1 {
		t.Fatalf("delta = %v, want 1", got)
	}
}
