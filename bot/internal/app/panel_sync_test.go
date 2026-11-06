package app

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/prometheus/client_golang/prometheus/testutil"

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
