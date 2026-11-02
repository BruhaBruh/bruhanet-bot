package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
)

// querier — общее подмножество pgx.Tx и *pgxpool.Pool.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func validate(m model.OutboxMessage) error {
	if m.EventType == "" || m.ChatID == 0 {
		return errors.Join(ErrEmptyMessage, fmt.Errorf("event_type=%q chat_id=%d", m.EventType, m.ChatID))
	}
	return nil
}

func enqueue(ctx context.Context, q querier, m model.OutboxMessage) error {
	if err := validate(m); err != nil {
		return err
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO outbox (event_type, payload) VALUES ($1, $2)`, m.EventType, payload)
	return err
}

func (s *Store) Enqueue(ctx context.Context, m model.OutboxMessage) error {
	return enqueue(ctx, s.pool, m)
}

func (s *Store) EnqueueMany(ctx context.Context, ms []model.OutboxMessage) error {
	for _, m := range ms { // до транзакции, чтобы не открывать её зря
		if err := validate(m); err != nil {
			return err
		}
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, m := range ms {
			if err := enqueue(ctx, tx, m); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) OutboxBatch(ctx context.Context, limit int) ([]model.OutboxItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, event_type, payload FROM outbox ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.OutboxItem
	for rows.Next() {
		var (
			it      model.OutboxItem
			payload []byte
		)
		if err = rows.Scan(&it.ID, &it.EventType, &payload); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(payload, &it.OutboxMessage); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) DeleteOutbox(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM outbox WHERE id = $1`, id)
	return err
}

func (s *Store) OutboxCount(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n)
	return n, err
}
