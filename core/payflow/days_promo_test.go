package payflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/promo"
)

type daysStore struct {
	refuse    error // отказ promo.Check до apply
	commitErr error // сбой коммита после apply
	activated int
}

func (s *daysStore) ActivateDaysPromo(_ context.Context, id, _ int64, _ time.Time, apply func(model.Promo) error) error {
	if s.refuse != nil {
		return s.refuse
	}
	if err := apply(model.Promo{ID: id, Kind: model.PromoDays, Value: 7}); err != nil {
		return err
	}
	if s.commitErr != nil {
		return s.commitErr
	}
	s.activated++
	return nil
}

// termPanel хранит срок единственного юзера и помнит записи.
type termPanel struct {
	expire    time.Time
	writes    []time.Time
	updateErr error
}

func (p *termPanel) GetUser(context.Context, int64) (*api.UserItemInfo, error) {
	return &api.UserItemInfo{ExpireAt: p.expire}, nil
}

func (p *termPanel) UpdateUser(_ context.Context, _ int64, _ string, at time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	if p.updateErr != nil {
		return nil, p.updateErr
	}
	p.expire = at
	p.writes = append(p.writes, at)
	return &api.UserItemInfo{ExpireAt: at}, nil
}

func daysReq(p *termPanel, ensureErr error) payflow.DaysPromoRequest {
	return payflow.DaysPromoRequest{
		PromoID: 5, TelegramID: 42, Username: "bruha", At: at,
		Ensure: func(ctx context.Context) (*api.UserItemInfo, error) {
			if ensureErr != nil {
				return nil, ensureErr
			}
			return p.GetUser(ctx, 42)
		},
	}
}

func TestActivateDaysPromo(t *testing.T) {
	day := 24 * time.Hour
	t.Run("extends from the current term", func(t *testing.T) {
		p, st := &termPanel{expire: at.Add(2 * day)}, &daysStore{}
		res, err := payflow.ActivateDaysPromo(t.Context(), st, p, daysReq(p, nil))
		if err != nil || !res.NewExpire.Equal(at.Add(9*day)) || !res.OldExpire.Equal(at.Add(2*day)) || res.Promo.Value != 7 {
			t.Fatalf("res=%+v err=%v", res, err)
		}
		if len(p.writes) != 1 || !p.writes[0].Equal(at.Add(9*day)) || st.activated != 1 {
			t.Fatalf("writes=%v activated=%d", p.writes, st.activated)
		}
	})
	t.Run("expired term counts from now", func(t *testing.T) {
		p := &termPanel{expire: at.Add(-10 * day)}
		res, err := payflow.ActivateDaysPromo(t.Context(), &daysStore{}, p, daysReq(p, nil))
		if err != nil || !res.NewExpire.Equal(at.Add(7*day)) {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})
	t.Run("store refusal passes through without panel writes", func(t *testing.T) {
		p := &termPanel{expire: at}
		_, err := payflow.ActivateDaysPromo(t.Context(), &daysStore{refuse: promo.ErrBlocked}, p, daysReq(p, nil))
		if !errors.Is(err, promo.ErrBlocked) || len(p.writes) != 0 {
			t.Fatalf("err=%v writes=%v", err, p.writes)
		}
	})
	t.Run("panel failure is ErrPanel and nothing is activated", func(t *testing.T) {
		p, st := &termPanel{expire: at, updateErr: errors.New("boom")}, &daysStore{}
		_, err := payflow.ActivateDaysPromo(t.Context(), st, p, daysReq(p, nil))
		if !errors.Is(err, payflow.ErrPanel) || st.activated != 0 {
			t.Fatalf("err=%v activated=%d", err, st.activated)
		}
	})
	t.Run("ensure failure is returned as is", func(t *testing.T) {
		p, boom := &termPanel{expire: at}, errors.New("no user")
		_, err := payflow.ActivateDaysPromo(t.Context(), &daysStore{}, p, daysReq(p, boom))
		if !errors.Is(err, boom) || len(p.writes) != 0 {
			t.Fatalf("err=%v writes=%v", err, p.writes)
		}
	})
	t.Run("commit failure restores the previous term", func(t *testing.T) {
		old := at.Add(2 * day)
		p, commit := &termPanel{expire: old}, errors.New("commit failed")
		_, err := payflow.ActivateDaysPromo(t.Context(), &daysStore{commitErr: commit}, p, daysReq(p, nil))
		if !errors.Is(err, commit) || !p.expire.Equal(old) {
			t.Fatalf("err=%v panel expire=%v want %v", err, p.expire, old)
		}
	})
}
