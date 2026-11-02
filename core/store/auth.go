package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateLoginToken(ctx context.Context, hash []byte, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO login_tokens (token_hash, expires_at) VALUES ($1, $2)`, hash, expiresAt)
	return err
}

// ConsumeLoginToken гасит токен; неизвестный, использованный или истёкший — ErrTokenInvalid.
func (s *Store) ConsumeLoginToken(ctx context.Context, hash []byte, now time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE login_tokens SET used_at = $2 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2`, hash, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenInvalid
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, hash []byte, telegramID int64, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id_hash, telegram_id, expires_at) VALUES ($1, $2, $3)`, hash, telegramID, expiresAt)
	return err
}

// SessionTelegramID — нет или истекла: ErrNotFound.
func (s *Store) SessionTelegramID(ctx context.Context, hash []byte, now time.Time) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT telegram_id FROM sessions WHERE id_hash = $1 AND expires_at > $2`, hash, now).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

func (s *Store) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id_hash = $1`, hash)
	return err
}

// PurgeAuth удаляет истёкшие токены и сессии.
func (s *Store) PurgeAuth(ctx context.Context, now time.Time) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM login_tokens WHERE expires_at <= $1`, now); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now)
	return err
}
