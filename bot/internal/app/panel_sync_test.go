package app

import (
	"context"
	"errors"
	"iter"
	"slices"
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
	// часы срабатываний всех задач
	hours := func() []int {
		hs := make([]int, 0, len(c.Entries()))
		for _, e := range c.Entries() {
			hs = append(hs, e.Schedule.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, msk)).Hour())
		}
		slices.Sort(hs)
		return hs
	}

	if err := j.reschedule([]string{"09:05", "12:00", "18:00"}); err != nil {
		t.Fatal(err)
	}
	if got := hours(); !slices.Equal(got, []int{9, 12, 18}) {
		t.Fatalf("hours = %v", got)
	}
	if err := j.reschedule([]string{"09:05", "12:00", "18:00"}); err != nil || len(c.Entries()) != 3 {
		t.Fatalf("same times: err = %v, entries = %d", err, len(c.Entries()))
	}
	if err := j.reschedule([]string{"18:30"}); err != nil {
		t.Fatal(err)
	}
	if got := hours(); !slices.Equal(got, []int{18}) {
		t.Fatalf("old jobs not removed: %v", got)
	}
	// битое время: прежняя задача остаётся, новые не повисают
	if err := j.reschedule([]string{"11:00", "мусор"}); err == nil || !slices.Equal(j.at, []string{"18:30"}) || len(c.Entries()) != 1 {
		t.Fatalf("err = %v, at = %q, entries = %d", err, j.at, len(c.Entries()))
	}
}

func TestReminderJobFollow(t *testing.T) {
	c := cron.New()
	j := &reminderJob{c: c, run: func() {}}
	if err := j.reschedule([]string{"09:00"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	moved := make(chan struct{})
	want := []string{"10:00", "15:00"}
	go func() {
		defer close(moved)
		j.follow(ctx, time.Millisecond, func(context.Context) []string {
			if slices.Equal(j.at, want) {
				cancel()
			}
			return want
		})
	}()
	<-moved
	if !slices.Equal(j.at, want) || len(c.Entries()) != 2 {
		t.Fatalf("at = %q, entries = %d", j.at, len(c.Entries()))
	}
}
