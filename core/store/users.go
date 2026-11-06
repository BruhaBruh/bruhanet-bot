package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

// TouchUser заводит юзера при первом апдейте и обновляет имя и время визита;
// написавший снова юзер точно не блокирует бота, поэтому blocked_bot_at снимается.
func (s *Store) TouchUser(ctx context.Context, u model.UserSeen) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (telegram_id, username, first_name, last_name, last_seen_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5)
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
			last_seen_at = EXCLUDED.last_seen_at, blocked_bot_at = NULL`,
		u.TelegramID, u.Username, u.FirstName, u.LastName, u.At)
	return err
}

// SetUserPanel записывает срок и статус сразу после вызова панели. Строку под блокировкой не ждёт и не трогает:
// вызов панели идёт и внутри транзакций (CreatePayment держит эту строку и ждёт панель, ожидание было бы
// дедлоком). Пропущенное дочинит синхронизация, строки ещё нет — тоже: её заведёт следующий шаг или TouchUser.
func (s *Store) SetUserPanel(ctx context.Context, id int64, expireAt time.Time, status string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET expire_at = $2, panel_status = $3, synced_at = $4
		WHERE telegram_id = (SELECT telegram_id FROM users WHERE telegram_id = $1 FOR NO KEY UPDATE SKIP LOCKED)`,
		id, expireAt, status, at)
	return err
}

// WithPanelSyncLock выполняет fn под advisory-блокировкой на отдельном соединении; уже занято — ErrSyncRunning.
// Ключ включает схему: тестовые схемы не мешают друг другу.
func (s *Store) WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	const key = `hashtext('panel_sync:' || current_schema())`
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(`+key+`)`).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return ErrSyncRunning
	}
	defer func() {
		// ctx мог истечь; не снялась — закрываем соединение, блокировка сессионная и уйдёт с ним.
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if _, err := conn.Exec(uctx, `SELECT pg_advisory_unlock(`+key+`)`); err != nil {
			_ = conn.Conn().Close(uctx)
		}
	}()
	return fn(ctx)
}

// ApplyPanelSync одной транзакцией заводит и обновляет юзеров панели, снимает срок и статус с тех, кого
// в панели больше нет, и запоминает итог. Строки, записанные после started (SetUserPanel), свежее снимка
// панели и не трогаются. Возвращает число записанных строк.
func (s *Store) ApplyPanelSync(ctx context.Context, users []model.PanelUser, started, finished time.Time) (int, error) {
	n := len(users)
	ids, names, statuses := make([]int64, n), make([]string, n), make([]string, n)
	expires, created := make([]time.Time, n), make([]time.Time, n)
	for i, u := range users {
		ids[i], names[i], statuses[i], expires[i], created[i] = u.TelegramID, u.Username, u.Status, u.ExpireAt, u.CreatedAt
	}
	var updated int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO users (telegram_id, username, panel_status, expire_at, created_at, synced_at)
			SELECT t.*, $6::timestamptz FROM unnest($1::bigint[], $2::text[], $3::text[], $4::timestamptz[], $5::timestamptz[]) AS t
			ON CONFLICT (telegram_id) DO UPDATE SET
				panel_status = EXCLUDED.panel_status, expire_at = EXCLUDED.expire_at, synced_at = EXCLUDED.synced_at
			WHERE users.synced_at IS NULL OR users.synced_at < $7`,
			ids, names, statuses, expires, created, finished, started)
		if err != nil {
			return err
		}
		updated = tag.RowsAffected()
		tag, err = tx.Exec(ctx, `
			UPDATE users SET panel_status = NULL, expire_at = NULL, synced_at = $2
			WHERE panel_status IS NOT NULL AND telegram_id <> ALL($1) AND (synced_at IS NULL OR synced_at < $3)`,
			ids, finished, started)
		if err != nil {
			return err
		}
		updated += tag.RowsAffected()
		_, err = tx.Exec(ctx, `
			INSERT INTO panel_sync (finished_at, total, updated) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET finished_at = EXCLUDED.finished_at, total = EXCLUDED.total, updated = EXCLUDED.updated`,
			finished, n, updated)
		return err
	})
	return int(updated), err
}

// LastPanelSync — итог последней успешной синхронизации; не было — ErrNotFound.
func (s *Store) LastPanelSync(ctx context.Context) (model.PanelSync, error) {
	var r model.PanelSync
	err := s.pool.QueryRow(ctx, `SELECT total, updated, finished_at FROM panel_sync`).Scan(&r.Total, &r.Updated, &r.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return r, err
}
