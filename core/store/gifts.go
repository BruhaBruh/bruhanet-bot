package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

// ErrGiftUnavailable — см. model.ErrGiftUnavailable.
var ErrGiftUnavailable = model.ErrGiftUnavailable

// ErrGiftNotActive — отменить можно только подарок в статусе ready.
var ErrGiftNotActive = errors.New("gift is not ready")

// NewGiftCode — случайный код подарка: 128 бит из crypto/rand в base64url (22 символа, без дополнения).
// Влезает в payload ссылки t.me/<бот>?start=gift_<код> (до 64 символов, A-Za-z0-9_-).
func NewGiftCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

const giftCols = `g.id, g.code, g.months, g.status, g.buyer_id, coalesce(b.username, ''), trim(concat_ws(' ', b.first_name, b.last_name)), g.payment_id, g.created_by_admin,
	g.redeemed_by, coalesce(r.username, ''), trim(concat_ws(' ', r.first_name, r.last_name)), g.redeemed_at, g.created_at`

const giftFrom = ` FROM gifts g LEFT JOIN users b ON b.telegram_id = g.buyer_id LEFT JOIN users r ON r.telegram_id = g.redeemed_by`

func scanGift(row pgx.Row) (g model.Gift, err error) {
	err = row.Scan(&g.ID, &g.Code, &g.Months, &g.Status, &g.BuyerID, &g.BuyerName, &g.BuyerFullName, &g.PaymentID, &g.ByAdmin,
		&g.RedeemedBy, &g.RedeemedName, &g.RedeemedFullName, &g.RedeemedAt, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return g, err
}

// insertPendingGift — подарок покупки: ждёт подтверждения платежа paymentID.
func insertPendingGift(ctx context.Context, tx pgx.Tx, buyer int64, paymentID int64, months int) error {
	code, err := NewGiftCode()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO gifts (code, months, status, buyer_id, payment_id) VALUES ($1, $2, 'pending', $3, $4)`,
		code, months, buyer, paymentID)
	return err
}

// CreateAdminGifts создаёт count подарков от админа на months месяцев сразу в статусе ready.
func (s *Store) CreateAdminGifts(ctx context.Context, months, count int, at time.Time) ([]model.Gift, error) {
	out := make([]model.Gift, 0, count)
	err := s.tx(ctx, func(tx pgx.Tx) error {
		out = out[:0]
		for range count {
			code, err := NewGiftCode()
			if err != nil {
				return err
			}
			g, err := scanGift(tx.QueryRow(ctx, `WITH g AS (
				INSERT INTO gifts (code, months, status, created_by_admin, created_at) VALUES ($1, $2, 'ready', true, $3) RETURNING *)
				SELECT `+giftCols+` FROM g `+giftJoins, code, months, at))
			if err != nil {
				return err
			}
			out = append(out, g)
		}
		return nil
	})
	return out, err
}

// giftJoins — те же join'ы для CTE с алиасом g.
const giftJoins = `g LEFT JOIN users b ON b.telegram_id = g.buyer_id LEFT JOIN users r ON r.telegram_id = g.redeemed_by`

// CancelGift отменяет подарок в статусе ready: ссылка перестаёт работать. Нет подарка — ErrNotFound,
// другой статус — ErrGiftNotActive (активированный подарок отменить нельзя).
func (s *Store) CancelGift(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var st model.GiftStatus
		if err := tx.QueryRow(ctx, `SELECT status FROM gifts WHERE id = $1 FOR UPDATE`, id).Scan(&st); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if st != model.GiftReady {
			return ErrGiftNotActive
		}
		_, err := tx.Exec(ctx, `UPDATE gifts SET status = 'cancelled' WHERE id = $1`, id)
		return err
	})
}

// DeleteGift удаляет подарок в статусе ready (отзыв и удаление одним шагом) или cancelled. Подарок админа
// (payment_id пуст) стирается физически; купленный остаётся строкой со статусом cancelled и deleted_at — след денег
// и связь с платежом. pending (ждёт оплаты) и redeemed (история, grants) — ErrNotDeletable. Строка FOR UPDATE,
// как у RedeemGift: параллельная активация либо выиграла (тогда отказ), либо увидит отозванный подарок.
// Повтор на удалённом — без ошибки; нет подарка — ErrNotFound.
func (s *Store) DeleteGift(ctx context.Context, id int64) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var (
			st      model.GiftStatus
			paid    bool
			deleted bool
		)
		if err := tx.QueryRow(ctx, `SELECT status, payment_id IS NOT NULL, deleted_at IS NOT NULL FROM gifts WHERE id = $1 FOR UPDATE`, id).
			Scan(&st, &paid, &deleted); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		switch {
		case deleted:
			return nil
		case st != model.GiftReady && st != model.GiftCancelled:
			return ErrNotDeletable
		case paid:
			_, err := tx.Exec(ctx, `UPDATE gifts SET status = 'cancelled', deleted_at = now() WHERE id = $1`, id)
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM gifts WHERE id = $1`, id)
		return err
	})
}

// ListGifts — страница (удалённые скрыты) подарков, новые первыми, и общее число. Код pending-подарка не отдаётся.
func (s *Store) ListGifts(ctx context.Context, f model.GiftFilter) ([]model.Gift, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM gifts WHERE deleted_at IS NULL AND ($1 = '' OR status = $1)`, string(f.Status)).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+giftCols+giftFrom+` WHERE g.deleted_at IS NULL AND ($1 = '' OR g.status = $1) ORDER BY g.id DESC LIMIT $2 OFFSET $3`,
		string(f.Status), f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Gift{}
	for rows.Next() {
		g, err := scanGift(rows)
		if err != nil {
			return nil, 0, err
		}
		if g.Status == model.GiftPending {
			g.Code = ""
		}
		out = append(out, g)
	}
	return out, total, rows.Err()
}

// RedeemGift активирует подарок code за юзера redeemerID одной транзакцией. Порядок блокировок как у GrantDays: замок
// срока юзера, затем строка подарка (FOR UPDATE: двое с одной ссылкой — выигрывает один). Любая причина отказа (нет
// кода, не ready, юзер заблокирован админом) — один и тот же ErrGiftUnavailable. Дальше: сдвиг базы pending-тарифа
// юзера на те же дни (иначе подтверждение или отклонение затрёт подарок), apply (продление в панели; вернуть уведомления
// для outbox), статус redeemed, строка grants (source gift, by_admin_id пуст). Ошибка apply откатывает всё.
func (s *Store) RedeemGift(
	ctx context.Context, code string, redeemerID int64, now time.Time, apply func(g model.Gift) ([]model.OutboxMessage, error),
) (model.Gift, error) {
	var g model.Gift
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockUserTerm(ctx, tx, redeemerID); err != nil {
			return err
		}
		var err error
		g, err = scanGift(tx.QueryRow(ctx, `SELECT `+giftCols+giftFrom+` WHERE g.code = $1 FOR UPDATE OF g`, code))
		if errors.Is(err, ErrNotFound) {
			g.Status = "" // ниже тот же отказ, что и у любой другой причины
		} else if err != nil {
			return err
		}
		blocked, err := userBlocked(ctx, tx, redeemerID)
		if err != nil {
			return err
		}
		if blocked || g.Status != model.GiftReady {
			return ErrGiftUnavailable
		}
		days := g.Months * 30
		if _, err = tx.Exec(ctx,
			`UPDATE payments SET prev_expire_at = greatest(prev_expire_at, $2) + make_interval(days => $3)
			 WHERE telegram_id = $1 AND status = 'pending' AND kind = 'tariff'`, redeemerID, now, days); err != nil {
			return err
		}
		msgs, err := apply(g)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE gifts SET status = 'redeemed', redeemed_by = $2, redeemed_at = $3 WHERE id = $1`,
			g.ID, redeemerID, now); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO grants (telegram_id, days, created_at, source) VALUES ($1, $2, $3, 'gift')`,
			redeemerID, days, now); err != nil {
			return err
		}
		for _, m := range msgs {
			if err = enqueue(ctx, tx, m); err != nil {
				return err
			}
		}
		g.Status, g.RedeemedBy, g.RedeemedAt = model.GiftRedeemed, &redeemerID, &now
		return nil
	})
	return g, err
}
