package app

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/robfig/cron/v3"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
)

type syncPanelFake struct{ err error }

func (p syncPanelFake) Users(context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) { yield(api.UserItemInfo{}, p.err) }
}

type syncStoreFake struct {
	lockErr error
	applied int
}

func (s *syncStoreFake) WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error {
	if s.lockErr != nil {
		return s.lockErr
	}
	return fn(ctx)
}

func (s *syncStoreFake) ApplyPanelSync(context.Context, []model.PanelUser, time.Time, time.Time) (int, error) {
	s.applied++
	return 0, nil
}

func TestSyncPanelCountsOnlyRealFailures(t *testing.T) {
	failures := func() float64 { return testutil.ToFloat64(observability.PanelSyncFailures) }
	before := failures()

	st := &syncStoreFake{}
	syncPanel(t.Context(), syncPanelFake{err: errors.New("panel down")}, st)
	if failures() != before+1 || st.applied != 0 {
		t.Fatalf("panel down: failures %v→%v applied=%d", before, failures(), st.applied)
	}

	syncPanel(t.Context(), syncPanelFake{err: errors.New("unused")}, &syncStoreFake{lockErr: store.ErrSyncRunning})
	if failures() != before+1 {
		t.Fatalf("busy lock counted as failure: %v", failures())
	}
}

func TestReminderJobReschedule(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	c := cron.New(cron.WithLocation(msk))
	j := &reminderJob{c: c, run: func() {}}
	next := func() time.Time { return c.Entries()[0].Schedule.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, msk)) }

	if err := j.reschedule("09:05"); err != nil {
		t.Fatal(err)
	}
	if n := next(); n.Hour() != 9 || n.Minute() != 5 {
		t.Fatalf("next = %v", n)
	}
	if err := j.reschedule("18:30"); err != nil {
		t.Fatal(err)
	}
	if len(c.Entries()) != 1 {
		t.Fatalf("entries = %d, старая задача не снята", len(c.Entries()))
	}
	if n := next(); n.Hour() != 18 || n.Minute() != 30 {
		t.Fatalf("next = %v", n)
	}
	if err := j.reschedule("мусор"); err == nil || j.at != "18:30" || len(c.Entries()) != 1 {
		t.Fatalf("err = %v, at = %q", err, j.at)
	}
}

func TestReminderJobFollow(t *testing.T) {
	c := cron.New()
	j := &reminderJob{c: c, run: func() {}}
	if err := j.reschedule("09:00"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	moved := make(chan struct{})
	go func() {
		defer close(moved)
		j.follow(ctx, time.Millisecond, func(context.Context) string {
			if j.at == "10:00" {
				cancel()
			}
			return "10:00"
		})
	}()
	<-moved
	if j.at != "10:00" {
		t.Fatalf("at = %q", j.at)
	}
}
