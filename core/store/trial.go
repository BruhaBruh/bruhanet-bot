package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

// ErrTrialUnavailable — см. model.ErrTrialUnavailable.
var ErrTrialUnavailable = model.ErrTrialUnavailable

// trialOpen — SQL-условие «юзер взял пробный и ещё не оплатил подписку»; alias — таблица users в запросе.
// Тот же признак даёт один неоплаченный аванс в UserStats и сбрасывается подтверждённой подпиской.
func trialOpen(alias string) string {
	return `(` + alias + `.trial_used_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM payments tp
		WHERE tp.telegram_id = ` + alias + `.telegram_id AND tp.status = 'confirmed' AND tp.kind = 'tariff'))`
}

// trialAllowed проверяет, что пробный по данным БД возможен: юзер не заблокирован админом, не брал его, не платил
// ни разу и не выбрал лимит авансов (пробный считается одним авансом). Юзера ещё нет — подходит.
// Срок в панели проверяет вызывающий: его читают под тем же замком (apply).
func trialAllowed(ctx context.Context, q rowQuerier, telegramID int64, advanceLimit int) error {
	var blocked, used, paid bool
	err := q.QueryRow(ctx, `SELECT
			coalesce((SELECT blocked_at IS NOT NULL FROM users WHERE telegram_id = $1), false),
			coalesce((SELECT trial_used_at IS NOT NULL FROM users WHERE telegram_id = $1), false),
			EXISTS (SELECT 1 FROM payments WHERE telegram_id = $1 AND status = 'confirmed')`, telegramID).Scan(&blocked, &used, &paid)
	if err != nil {
		return err
	}
	if blocked || used || paid {
		return ErrTrialUnavailable
	}
	st, err := userStats(ctx, q, telegramID, 0)
	if err != nil {
		return err
	}
	if advanceLimit > 0 && st.UnpaidAdvances >= advanceLimit {
		return ErrTrialUnavailable
	}
	return nil
}

// TrialAllowed — дешёвая проверка для меню; решает проверка внутри StartTrial.
func (s *Store) TrialAllowed(ctx context.Context, telegramID int64, advanceLimit int) (bool, error) {
	err := trialAllowed(ctx, s.pool, telegramID, advanceLimit)
	if errors.Is(err, ErrTrialUnavailable) {
		return false, nil
	}
	return err == nil, err
}

// StartTrial выдаёт пробный период одной транзакцией под замком срока юзера: проверка по БД, сдвиг базы pending-платежа
// на те же дни (как GrantDays), apply (панель: проверить срок, продлить, записать лимит; вернуть уведомления для outbox),
// trial_used_at и строка grants (source trial). Ошибка apply откатывает всё; недоступно — ErrTrialUnavailable.
// apply сам возвращает ErrTrialUnavailable, если юзер уже есть в панели с оплаченным сроком.
func (s *Store) StartTrial(
	ctx context.Context, telegramID int64, username string, days, advanceLimit int, now time.Time,
	apply func() ([]model.OutboxMessage, error),
) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockUserTerm(ctx, tx, telegramID); err != nil {
			return err
		}
		// Строку юзера не блокируем (запись срока в панель SKIP LOCKED): DO NOTHING её не лочит, если она есть.
		if _, err := tx.Exec(ctx, `INSERT INTO users (telegram_id, username) VALUES ($1, $2)
			ON CONFLICT (telegram_id) DO NOTHING`, telegramID, username); err != nil {
			return err
		}
		if err := trialAllowed(ctx, tx, telegramID, advanceLimit); err != nil {
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
		if _, err = tx.Exec(ctx, `UPDATE users SET trial_used_at = $2 WHERE telegram_id = $1`, telegramID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO grants (telegram_id, days, created_at, source) VALUES ($1, $2, $3, 'trial')`,
			telegramID, days, now); err != nil {
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
