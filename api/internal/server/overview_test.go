package server_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

// UserGrowth считает по users.created_at: окна [prev, week) и [week, end).
func (f *fakeStore) UserGrowth(_ context.Context, prevFrom, weekFrom, to time.Time) (total, week, prev int, err error) {
	if f.growthErr != nil {
		return 0, 0, 0, f.growthErr
	}
	for _, u := range f.users {
		total++
		switch {
		case !u.CreatedAt.Before(weekFrom) && u.CreatedAt.Before(to):
			week++
		case !u.CreatedAt.Before(prevFrom) && u.CreatedAt.Before(weekFrom):
			prev++
		}
	}
	return total, week, prev, nil
}

func (f *fakeStore) CountPromoActivations(_ context.Context, from, to time.Time) (int, error) {
	n := 0
	for _, a := range f.acts {
		if !a.CreatedAt.Before(from) && a.CreatedAt.Before(to) {
			n++
		}
	}
	return n, nil
}

type overviewJSON struct {
	Users *struct {
		Total int `json:"total"`
		New7  int `json:"new7"`
		Prev7 int `json:"prev7"`
	} `json:"users"`
	Promos *struct {
		Active      int `json:"active"`
		Activations int `json:"activations7"`
	} `json:"promos"`
	Pending *int `json:"pending"`
	Revenue *struct {
		Week int `json:"week"`
		Prev int `json:"prevWeek"`
	} `json:"revenue"`
}

func TestOverview(t *testing.T) {
	h, st, c := promoServer(t)
	// now = 2026-11-02 15:00 МСК; неделя = [27 окт 00:00 МСК, 3 ноя 00:00 МСК), прошлая — 20..26 окт
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	st.users = []model.UserSummary{
		{TelegramID: 1, CreatedAt: day(20)},
		{TelegramID: 2, CreatedAt: day(26)},
		{TelegramID: 3, CreatedAt: day(19)},
		{TelegramID: 4, CreatedAt: day(27)},
		{TelegramID: 5, CreatedAt: now},
	}
	st.promos = []model.PromoWithUsage{
		{Promo: model.Promo{ID: 1, Code: "A", Kind: model.PromoDays}}, {Promo: model.Promo{ID: 2, Code: "B", Kind: model.PromoDays, Disabled: true}},
	}
	st.acts = []model.PromoActivation{{CreatedAt: day(28)}, {CreatedAt: day(10)}, {CreatedAt: now}}
	st.payments = []model.Payment{{ID: 1}, {ID: 2}, {ID: 3}}
	msk := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.FixedZone("MSK", 3*3600)) }
	st.revFn = func(from, to time.Time) (int, error) {
		if from.Equal(msk(27)) && to.Equal(msk(27).AddDate(0, 0, 7)) {
			return 900, nil
		}
		if from.Equal(msk(20)) && to.Equal(msk(27)) {
			return 600, nil
		}
		t.Errorf("unexpected revenue window %v..%v", from, to)
		return 0, nil
	}

	var out overviewJSON
	getJSON(t, h, "/api/overview", c, &out)
	if out.Users == nil || *out.Users != (struct {
		Total int `json:"total"`
		New7  int `json:"new7"`
		Prev7 int `json:"prev7"`
	}{5, 2, 2}) {
		t.Fatalf("users = %+v", out.Users)
	}
	if out.Promos == nil || out.Promos.Active != 1 || out.Promos.Activations != 2 {
		t.Fatalf("promos = %+v", out.Promos)
	}
	if out.Pending == nil || *out.Pending != 3 || st.lastF.Status != model.PaymentPending {
		t.Fatalf("pending = %v, filter %+v", out.Pending, st.lastF)
	}
	if out.Revenue == nil || out.Revenue.Week != 900 || out.Revenue.Prev != 600 {
		t.Fatalf("revenue = %+v", out.Revenue)
	}
}

// Ошибка одной секции — null только у неё, остальные считаются, ответ 200.
func TestOverviewSectionErrorsAreIsolated(t *testing.T) {
	h, st, c := promoServer(t)
	st.growthErr = errors.New("boom")
	st.listErr = errors.New("boom")
	var out overviewJSON
	getJSON(t, h, "/api/overview", c, &out)
	if out.Users != nil || out.Pending != nil {
		t.Fatalf("failed sections must be null: %+v", out)
	}
	if out.Promos == nil || out.Revenue == nil {
		t.Fatalf("healthy sections lost: %+v", out)
	}
}

func TestOverviewRequiresSession(t *testing.T) {
	h, _, _ := promoServer(t)
	if w := do(h, http.MethodGet, "/api/overview", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d", w.Code)
	}
}
