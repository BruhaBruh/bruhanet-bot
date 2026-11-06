package referral

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
)

// fakeStore: награды due; PayReferrer зовёт GrantReferral, ошибка по id управляется тестом.
type fakeStore struct {
	due   []int64
	errs  map[int64]error
	calls map[int64]int
	skips [][]int64
}

func (f *fakeStore) ReferralsDue(_ context.Context, skip []int64, limit int) ([]int64, error) {
	f.skips = append(f.skips, append([]int64{}, skip...))
	var out []int64
	for _, id := range f.due {
		skipped := false
		for _, s := range skip {
			skipped = skipped || s == id
		}
		if !skipped && len(out) < limit {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeStore) GrantReferral(_ context.Context, id int64, _ time.Time, _ func(int64, string, int) ([]model.OutboxMessage, error)) error {
	f.calls[id]++
	if err := f.errs[id]; err != nil {
		return err
	}
	for i, d := range f.due {
		if d == id {
			f.due = append(f.due[:i], f.due[i+1:]...)
			break
		}
	}
	return nil
}

func TestReconcilerBackoffDoesNotStarveOthers(t *testing.T) {
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	f := &fakeStore{due: []int64{1, 2, 3}, errs: map[int64]error{1: errors.New("boom")}, calls: map[int64]int{}}
	r := New(f, nil, uuid.Nil, func() time.Time { return at })

	r.Run(t.Context())
	if f.calls[1] != 1 || f.calls[2] != 1 || f.calls[3] != 1 || len(f.due) != 1 {
		t.Fatalf("calls %v due %v", f.calls, f.due)
	}
	// сразу же: упавшая откладывается, лишних вызовов нет
	r.Run(t.Context())
	if f.calls[1] != 1 {
		t.Fatalf("упавшая награда повторена без паузы: %v", f.calls)
	}
	// после паузы — снова; пауза растёт
	at = at.Add(backoffBase + time.Second)
	r.Run(t.Context())
	if f.calls[1] != 2 {
		t.Fatalf("повтор после паузы: %v", f.calls)
	}
	at = at.Add(backoffBase + time.Second) // вторая пауза в 2 раза длиннее
	r.Run(t.Context())
	if f.calls[1] != 2 {
		t.Fatalf("вторая пауза не выросла: %v", f.calls)
	}
	// ошибка ушла
	f.errs[1] = nil
	at = at.Add(backoffBase * 2)
	r.Run(t.Context())
	if len(f.due) != 0 {
		t.Fatalf("награда не выдана: %v", f.due)
	}
}

func TestReconcilerSentinels(t *testing.T) {
	at := time.Now()
	f := &fakeStore{due: []int64{1, 2, 3}, calls: map[int64]int{}, errs: map[int64]error{
		1: store.ErrAlreadyRewarded, 2: store.ErrBlocked, 3: store.ErrNotFound,
	}}
	r := New(f, nil, uuid.Nil, func() time.Time { return at })
	r.Run(t.Context())
	// не сбои: в бэкофф не попадают
	if got := r.skip(at); len(got) != 0 {
		t.Fatalf("skip = %v", got)
	}
}
