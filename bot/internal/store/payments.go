package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
)

const paymentCols = `id, telegram_id, username, months, price, status, prev_expire_at, advance_granted,
	coalesce(admin_message_id, 0), created_at, submitted_at, resolved_at`

func scanPayment(row pgx.Row) (model.Payment, error) {
	var p model.Payment
	err := row.Scan(&p.ID, &p.TelegramID, &p.Username, &p.Month, &p.Price, &p.Status, &p.PrevExpireAt,
		&p.AdvanceGranted, &p.AdminMessageID, &p.CreatedAt, &p.SubmittedAt, &p.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// CreatePayment создаёт pending-платёж. Если у юзера уже есть pending, он отменяется
// (rollback вызывается под блокировкой строки, ошибка откатывает всё), а PrevExpireAt берётся из него.
func (s *Store) CreatePayment(
	ctx context.Context, p model.Payment, rollback func(old model.Payment) error,
) (created model.Payment, cancelled *model.Payment, err error) {
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// upsert берёт row-lock на users, поэтому параллельные CreatePayment одного юзера сериализуются.
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (telegram_id, username) VALUES ($1, $2)
			 ON CONFLICT (telegram_id) DO UPDATE SET username = EXCLUDED.username`,
			p.TelegramID, p.Username); err != nil {
			return err
		}

		old, err := scanPayment(tx.QueryRow(ctx,
			`SELECT `+paymentCols+` FROM payments WHERE telegram_id = $1 AND status = 'pending' FOR UPDATE`,
			p.TelegramID))
		switch {
		case err == nil:
			if err = rollback(old); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx,
				`UPDATE payments SET status = 'cancelled', resolved_at = now() WHERE id = $1`, old.ID); err != nil {
				return err
			}
			old.Status = model.PaymentCancelled
			cancelled = &old
			p.PrevExpireAt = old.PrevExpireAt
		case !errors.Is(err, ErrNotFound):
			return err
		}

		created, err = scanPayment(tx.QueryRow(ctx,
			`INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at)
			 VALUES ($1, $2, $3, $4, 'pending', $5) RETURNING `+paymentCols,
			p.TelegramID, p.Username, p.Month, p.Price, p.PrevExpireAt))
		return err
	})
	if err != nil {
		return model.Payment{}, nil, err
	}
	return created, cancelled, nil
}

func (s *Store) PendingPayment(ctx context.Context, telegramID int64) (model.Payment, error) {
	return scanPayment(s.pool.QueryRow(ctx,
		`SELECT `+paymentCols+` FROM payments WHERE telegram_id = $1 AND status = 'pending'`, telegramID))
}

func (s *Store) PaymentByID(ctx context.Context, id int64) (model.Payment, error) {
	return scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1`, id))
}

// SubmitPayment фиксирует отправку чека; повторный вызов даёт ErrNotPending.
func (s *Store) SubmitPayment(ctx context.Context, id int64, adminMessageID int, at time.Time) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE payments SET admin_message_id = $2, submitted_at = $3
		 WHERE id = $1 AND status = 'pending' AND submitted_at IS NULL`, id, adminMessageID, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotPending
	}
	return nil
}

func (s *Store) MarkAdvance(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE payments SET advance_granted = true WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UserStats считает статистику юзера; UnpaidAdvances — выданные авансы после последнего confirmed.
func (s *Store) UserStats(ctx context.Context, telegramID, excludeID int64) (model.UserStats, error) {
	var st model.UserStats
	err := s.pool.QueryRow(ctx, `
		WITH p AS (SELECT * FROM payments WHERE telegram_id = $1 AND id <> $2),
		last AS (SELECT coalesce(max(id) FILTER (WHERE status = 'confirmed'), 0) AS id FROM p)
		SELECT
			count(*) FILTER (WHERE status = 'confirmed'),
			coalesce(sum(price) FILTER (WHERE status = 'confirmed'), 0),
			count(*) FILTER (WHERE status IN ('declined', 'cancelled') AND advance_granted AND p.id > last.id),
			max(coalesce(resolved_at, created_at)) FILTER (WHERE status = 'confirmed'),
			min(coalesce(resolved_at, created_at)) FILTER (WHERE status = 'confirmed'),
			count(*) > 0
		FROM p, last`, telegramID, excludeID).
		Scan(&st.Confirmed, &st.ConfirmedSum, &st.UnpaidAdvances, &st.LastConfirmedAt, &st.FirstConfirmedAt, &st.HasHistory)
	return st, err
}

// ResolvePayment под блокировкой строки вызывает apply и переводит pending в confirmed|declined;
// сообщение из apply кладётся в outbox в той же транзакции.
// При ErrNotPending возвращается и загруженный платёж.
func (s *Store) ResolvePayment(
	ctx context.Context, id int64, status model.PaymentStatus, at time.Time, apply func(p model.Payment) (model.OutboxMessage, error),
) (model.Payment, error) {
	if status != model.PaymentConfirmed && status != model.PaymentDeclined {
		return model.Payment{}, errors.Join(ErrInvalidStatus, fmt.Errorf("status %q", status))
	}
	var p model.Payment
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		p, err = scanPayment(tx.QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if p.Status != model.PaymentPending {
			return ErrNotPending
		}
		msg, err := apply(p)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE payments SET status = $2, resolved_at = $3 WHERE id = $1`,
			id, status, at); err != nil {
			return err
		}
		if err = enqueue(ctx, tx, msg); err != nil {
			return err
		}
		p.Status, p.ResolvedAt = status, &at
		return nil
	})
	return p, err
}
