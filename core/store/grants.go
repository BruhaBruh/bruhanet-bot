package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

// userBlocked — заблокирован ли юзер админом; нет юзера — нет.
func userBlocked(ctx context.Context, q rowQuerier, telegramID int64) (blocked bool, err error) {
	err = q.QueryRow(ctx, `SELECT coalesce((SELECT blocked_at IS NOT NULL FROM users WHERE telegram_id = $1), false)`,
		telegramID).Scan(&blocked)
	return blocked, err
}

// UserBlocked — для раннего отказа в боте; решает проверка под транзакцией выдачи.
func (s *Store) UserBlocked(ctx context.Context, telegramID int64) (bool, error) {
	return userBlocked(ctx, s.pool, telegramID)
}

// SetUserBlocked ставит (blocked) или снимает ручную блокировку; повтор ничего не меняет, время первой блокировки
// сохраняется. Нет юзера — ErrNotFound.
func (s *Store) SetUserBlocked(ctx context.Context, telegramID int64, blocked bool, at time.Time) error {
	return affected(s.pool.Exec(ctx, `
		UPDATE users SET blocked_at = CASE WHEN $2 THEN coalesce(blocked_at, $3) END WHERE telegram_id = $1`,
		telegramID, blocked, at))
}

// GrantDays выдаёт юзеру дни от админа одной транзакцией: сдвигает базу его pending-платежа
// на те же дни (как ActivateDaysPromo: иначе подтверждение или отклонение затрёт бонус), вызывает apply (продление
// в панели; вернуть уведомления юзеру для outbox) и пишет строку grants. Ошибка apply откатывает всё.
// Админская выдача работает и для заблокированного вручную юзера: блокировка запрещает бесплатное, что юзер получает
// сам, а не решение админа. Нет юзера — ErrNotFound.
func (s *Store) GrantDays(
	ctx context.Context, telegramID, adminID int64, days int, now time.Time, apply func() ([]model.OutboxMessage, error),
) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockUserTerm(ctx, tx, telegramID); err != nil {
			return err
		}
		// Строку юзера не блокируем: запись срока в панель (SetUserPanel) пропускает залоченные строки.
		// Подтверждение в полёте держит платёж: UPDATE ниже дождётся его коммита, срок в панели читается уже после.
		if _, err := scanUserID(tx.QueryRow(ctx, `SELECT telegram_id FROM users WHERE telegram_id = $1`, telegramID)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE payments SET prev_expire_at = greatest(prev_expire_at, $2) + make_interval(days => $3)
			 WHERE telegram_id = $1 AND status = 'pending'`, telegramID, now, days); err != nil {
			return err
		}
		msgs, err := apply()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO grants (telegram_id, days, created_at, by_admin_id) VALUES ($1, $2, $3, $4)`,
			telegramID, days, now, adminID); err != nil {
			return err
		}
		for _, m := range msgs {
			if err = enqueue(ctx, tx, m); err != nil {
				return err
			}
		}
		return nil
	})
}

func scanUserID(row pgx.Row) (id int64, err error) {
	if err = row.Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return id, err
}

// UserGrants — последние выдачи дней юзеру, новые первыми.
func (s *Store) UserGrants(ctx context.Context, telegramID int64, limit int) ([]model.Grant, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, days, created_at FROM grants WHERE telegram_id = $1 AND source = 'admin' ORDER BY id DESC LIMIT $2`,
		telegramID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Grant{}
	for rows.Next() {
		var g model.Grant
		if err = rows.Scan(&g.ID, &g.Days, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
