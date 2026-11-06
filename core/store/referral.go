package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

// RecordReferral заводит строку нового юзера сразу с referred_by: пригласивший должен уже быть в users и не быть
// самим юзером. Если строка юзера уже есть (он не новый), ничего не меняется и referred_by не перезаписывается.
// Возвращает true, если юзер создан этим вызовом. Вызывается из /start до того, как LastSeen завёл строку.
func (s *Store) RecordReferral(ctx context.Context, u model.UserSeen, referrerID int64) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO users (telegram_id, username, first_name, last_name, last_seen_at, referred_by)
		SELECT $1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, r.telegram_id FROM users r
		WHERE r.telegram_id = $6 AND r.telegram_id <> $1
		ON CONFLICT (telegram_id) DO NOTHING`,
		u.TelegramID, u.Username, u.FirstName, u.LastName, u.At, referrerID)
	return tag.RowsAffected() == 1, err
}

// GrantReferral выдаёт пригласившему его дни за друга referredID одной транзакцией под замком срока пригласившего:
// строка награды блокируется, referrer_rewarded_at проверяется под замком (повтор и параллельный запуск — ErrAlreadyRewarded),
// заблокированному админом пригласившему не выдаётся (ErrBlocked, строка остаётся для повтора после снятия блокировки),
// база его pending-платежа сдвигается на те же дни (как GrantDays), apply продлевает срок в панели и возвращает
// уведомления для outbox, пишутся grants (source referral) и referrer_rewarded_at. Ошибка apply откатывает всё.
// Нет награды или пригласившего — ErrNotFound.
func (s *Store) GrantReferral(
	ctx context.Context, referredID int64, now time.Time,
	apply func(referrerID int64, username string, days int) ([]model.OutboxMessage, error),
) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		// строка награды неизменна (referrer_id, days): читаем без замка, чтобы взять advisory-замок пригласившего первым
		var referrer int64
		var days int
		if err := tx.QueryRow(ctx, `SELECT referrer_id, days FROM referral_rewards WHERE referred_id = $1`, referredID).
			Scan(&referrer, &days); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := lockUserTerm(ctx, tx, referrer); err != nil {
			return err
		}
		var rewarded *time.Time
		if err := tx.QueryRow(ctx, `SELECT referrer_rewarded_at FROM referral_rewards WHERE referred_id = $1 FOR UPDATE`, referredID).
			Scan(&rewarded); err != nil {
			return err
		}
		if rewarded != nil {
			return ErrAlreadyRewarded
		}
		var username string
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT username, blocked_at IS NOT NULL FROM users WHERE telegram_id = $1`, referrer).
			Scan(&username, &blocked); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if blocked {
			return ErrBlocked
		}
		if _, err := tx.Exec(ctx,
			`UPDATE payments SET prev_expire_at = greatest(prev_expire_at, $2) + make_interval(days => $3)
			 WHERE telegram_id = $1 AND status = 'pending'`, referrer, now, days); err != nil {
			return err
		}
		msgs, err := apply(referrer, username, days)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE referral_rewards SET referrer_rewarded_at = $2 WHERE referred_id = $1`, referredID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO grants (telegram_id, days, created_at, source) VALUES ($1, $2, $3, 'referral')`,
			referrer, days, now); err != nil {
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

// ReferralsDue — приглашённые, за которых пригласивший ещё не получил дни (самые старые первыми). Заблокированные
// админом пригласившие не попадают: после снятия блокировки они вернутся сами. skip — приглашённые, которых
// вызывающий пока пропускает (повторяющиеся сбои).
func (s *Store) ReferralsDue(ctx context.Context, skip []int64, limit int) ([]int64, error) {
	if skip == nil {
		skip = []int64{} // <> ALL(NULL) не истинно ни для одной строки
	}
	rows, err := s.pool.Query(ctx, `
		SELECT r.referred_id FROM referral_rewards r JOIN users u ON u.telegram_id = r.referrer_id
		WHERE r.referrer_rewarded_at IS NULL AND u.blocked_at IS NULL AND r.referred_id <> ALL($1::bigint[])
		ORDER BY r.created_at, r.referred_id LIMIT $2`, skip, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UserReferral — рефералка юзера для карточки: сколько пригласил, сколько из них оплатили, кто пригласил его самого.
func (s *Store) UserReferral(ctx context.Context, id int64) (model.UserReferral, error) {
	var r model.UserReferral
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM users WHERE referred_by = $1),
		       (SELECT count(*) FROM referral_rewards WHERE referrer_id = $1),
		       u.referred_by, coalesce(ru.username, '')
		FROM users u LEFT JOIN users ru ON ru.telegram_id = u.referred_by WHERE u.telegram_id = $1`, id).
		Scan(&r.Invited, &r.Paid, &r.ReferredBy, &r.ReferredByUsername)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.UserReferral{}, ErrNotFound
	}
	return r, err
}
