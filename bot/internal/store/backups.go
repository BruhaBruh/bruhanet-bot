package store

import (
	"context"
	"time"
)

// LastBackupAt возвращает время последнего бэкапа; ErrNotFound, если бэкапов нет.
func (s *Store) LastBackupAt(ctx context.Context) (time.Time, error) {
	var t *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(created_at) FROM backups`).Scan(&t); err != nil {
		return time.Time{}, err
	}
	if t == nil {
		return time.Time{}, ErrNotFound
	}
	return *t, nil
}
