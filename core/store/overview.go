package store

import (
	"context"
	"time"
)

// UserGrowth — всего юзеров и новых (users.created_at) в окнах [weekFrom, to) и [prevFrom, weekFrom).
func (s *Store) UserGrowth(ctx context.Context, prevFrom, weekFrom, to time.Time) (total, week, prev int, err error) {
	err = s.pool.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER (WHERE created_at >= $2 AND created_at < $3),
			count(*) FILTER (WHERE created_at >= $1 AND created_at < $2)
		FROM users`, prevFrom, weekFrom, to).Scan(&total, &week, &prev)
	return total, week, prev, err
}

// CountPromoActivations — активации промокодов с created_at в [from, to).
func (s *Store) CountPromoActivations(ctx context.Context, from, to time.Time) (n int, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT count(*) FROM promo_activations WHERE created_at >= $1 AND created_at < $2`, from, to).Scan(&n)
	return n, err
}
