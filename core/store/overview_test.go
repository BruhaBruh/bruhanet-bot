package store

import (
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
)

func TestUserGrowthAndPromoActivationCounts(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	end := time.Now().Truncate(time.Hour).Add(time.Hour)
	week, prev := end.AddDate(0, 0, -7), end.AddDate(0, 0, -14)
	// created_at: на границах окон и вне них
	for id, at := range map[int64]time.Time{
		1: prev.Add(-time.Minute), 2: prev, 3: week.Add(-time.Second), 4: week, 5: end.Add(-time.Second), 6: end,
	} {
		if err := s.TouchUser(ctx, model.UserSeen{TelegramID: id, At: at}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE users SET created_at = $2 WHERE telegram_id = $1`, id, at); err != nil {
			t.Fatal(err)
		}
	}
	total, w, p, err := s.UserGrowth(ctx, prev, week, end)
	if err != nil || total != 6 || w != 2 || p != 2 {
		t.Fatalf("growth = %d %d %d %v", total, w, p, err)
	}

	pc := mustPromo(t, s, model.Promo{Code: "COUNTME", Kind: promo.KindDays, Value: 1})
	for id, at := range map[int64]time.Time{1: week.Add(-time.Second), 2: week, 3: end.Add(-time.Second), 4: end} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO promo_activations (promo_id, telegram_id, created_at) VALUES ($1, $2, $3)`, pc.ID, id, at); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.CountPromoActivations(ctx, week, end); err != nil || n != 2 {
		t.Fatalf("activations = %d %v", n, err)
	}
}
