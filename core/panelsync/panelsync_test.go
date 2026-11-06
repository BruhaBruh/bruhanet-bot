package panelsync

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/model"
)

type fakePanel struct {
	users []api.UserItemInfo
	err   error
}

func (p fakePanel) Users(context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		for _, u := range p.users {
			if !yield(u, nil) {
				return
			}
		}
		if p.err != nil {
			yield(api.UserItemInfo{}, p.err)
		}
	}
}

type fakeStore struct {
	lockErr error
	applied [][]model.PanelUser
}

func (s *fakeStore) WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error {
	if s.lockErr != nil {
		return s.lockErr
	}
	return fn(ctx)
}

func (s *fakeStore) ApplyPanelSync(_ context.Context, users []model.PanelUser, _, _ time.Time) (int, error) {
	s.applied = append(s.applied, users)
	return len(users), nil
}

func user(id int, name string) api.UserItemInfo {
	return api.UserItemInfo{Username: name, Status: api.UserItemInfoStatusACTIVE, TelegramId: api.NewNilInt(id)}
}

func TestRunDedupesAndSkipsUsersWithoutTelegram(t *testing.T) {
	st := &fakeStore{}
	noTG := user(0, "web")
	noTG.TelegramId = api.NilInt{Null: true}
	res, err := Run(t.Context(), fakePanel{users: []api.UserItemInfo{user(1, "ann"), noTG, user(1, "ann2"), user(2, "2")}}, st, time.Now)
	if err != nil || res.Total != 2 || res.Updated != 2 || res.FinishedAt.IsZero() {
		t.Fatalf("%+v %v", res, err)
	}
	got := st.applied[0]
	if len(got) != 2 || got[0].Username != "ann" || got[0].Status != "ACTIVE" || got[1].Username != "" {
		t.Fatalf("users = %+v", got)
	}
}

func TestRunPanelFailureWritesNothing(t *testing.T) {
	st := &fakeStore{}
	boom := errors.New("boom")
	_, err := Run(t.Context(), fakePanel{users: []api.UserItemInfo{user(1, "ann")}, err: boom}, st, time.Now)
	if !errors.Is(err, ErrPanel) || !errors.Is(err, boom) || len(st.applied) != 0 {
		t.Fatalf("err=%v applied=%d", err, len(st.applied))
	}
	_, err = Run(t.Context(), fakePanel{}, st, time.Now)
	if !errors.Is(err, ErrPanel) || len(st.applied) != 0 {
		t.Fatalf("empty: err=%v applied=%d", err, len(st.applied))
	}
}

func TestRunLockBusyDoesNotReadPanel(t *testing.T) {
	busy := errors.New("busy")
	if _, err := Run(t.Context(), fakePanel{err: errors.New("must not read")}, &fakeStore{lockErr: busy}, time.Now); !errors.Is(err, busy) || errors.Is(err, ErrPanel) {
		t.Fatalf("err=%v", err)
	}
}
