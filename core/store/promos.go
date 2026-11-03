package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
)

const promoCols = `id, code, kind, value, max_activations, expires_at, only_new, telegram_id, disabled, created_at`

func scanPromo(row pgx.Row, extra ...any) (model.Promo, error) {
	var p model.Promo
	err := row.Scan(append([]any{
		&p.ID, &p.Code, &p.Kind, &p.Value, &p.MaxActivations, &p.ExpiresAt,
		&p.OnlyNew, &p.TelegramID, &p.Disabled, &p.CreatedAt,
	}, extra...)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// CreatePromo сохраняет код в виде promo.Normalize; занятый код — ErrPromoExists.
// Валидация (promo.ValidateNew) — на вызывающем.
func (s *Store) CreatePromo(ctx context.Context, p model.Promo) (model.Promo, error) {
	created, err := scanPromo(s.pool.QueryRow(ctx,
		`INSERT INTO promo_codes (code, kind, value, max_activations, expires_at, only_new, telegram_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+promoCols,
		promo.Normalize(p.Code), p.Kind, p.Value, p.MaxActivations, p.ExpiresAt, p.OnlyNew, p.TelegramID))
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok && pg.Code == "23505" {
		return model.Promo{}, errors.Join(ErrPromoExists, err)
	}
	return created, err
}

// PromoByCode ищет без учёта регистра и пробелов по краям.
func (s *Store) PromoByCode(ctx context.Context, code string) (model.Promo, error) {
	return scanPromo(s.pool.QueryRow(ctx, `SELECT `+promoCols+` FROM promo_codes WHERE code = $1`, promo.Normalize(code)))
}

func (s *Store) PromoByID(ctx context.Context, id int64) (model.Promo, error) {
	return scanPromo(s.pool.QueryRow(ctx, `SELECT `+promoCols+` FROM promo_codes WHERE id = $1`, id))
}

// ListPromos — коды с числом активаций, новые первыми; activeOnly — не выключенные,
// не истёкшие к now и с оставшимися активациями.
func (s *Store) ListPromos(ctx context.Context, activeOnly bool, now time.Time) ([]model.PromoWithUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+promoCols+`, used FROM (
			SELECT c.*, (SELECT count(*) FROM promo_activations a WHERE a.promo_id = c.id)::int AS used
			FROM promo_codes c) c
		WHERE NOT $1 OR (NOT disabled AND (expires_at IS NULL OR expires_at > $2)
			AND (max_activations IS NULL OR used < max_activations))
		ORDER BY id DESC`, activeOnly, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PromoWithUsage{}
	for rows.Next() {
		var p model.PromoWithUsage
		if p.Promo, err = scanPromo(rows, &p.Used); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DisablePromo выключает код; неизвестный — ErrNotFound.
func (s *Store) DisablePromo(ctx context.Context, id int64) error {
	return affected(s.pool.Exec(ctx, `UPDATE promo_codes SET disabled = true WHERE id = $1`, id))
}

// PromoActivations — кто и когда активировал код, новые первыми.
func (s *Store) PromoActivations(ctx context.Context, id int64) ([]model.PromoActivation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.promo_id, a.telegram_id, coalesce(u.username, ''), a.payment_id, a.created_at
		FROM promo_activations a LEFT JOIN users u ON u.telegram_id = a.telegram_id
		WHERE a.promo_id = $1 ORDER BY a.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.PromoActivation{}
	for rows.Next() {
		var a model.PromoActivation
		if err = rows.Scan(&a.ID, &a.PromoID, &a.TelegramID, &a.Username, &a.PaymentID, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PromoUsage — всего активаций кода и активировал ли его этот юзер.
func (s *Store) PromoUsage(ctx context.Context, promoID, telegramID int64) (used int, alreadyActivated bool, err error) {
	return promoUsage(ctx, s.pool, promoID, telegramID)
}

// rowQuerier — пул или транзакция.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func promoUsage(ctx context.Context, q rowQuerier, promoID, telegramID int64) (used int, alreadyActivated bool, err error) {
	err = q.QueryRow(ctx, `SELECT count(*), coalesce(bool_or(telegram_id = $2), false)
		FROM promo_activations WHERE promo_id = $1`, promoID, telegramID).Scan(&used, &alreadyActivated)
	return used, alreadyActivated, err
}

// HasConfirmed — есть ли у юзера подтверждённая оплата («только новым»).
func (s *Store) HasConfirmed(ctx context.Context, telegramID int64) (bool, error) {
	return hasConfirmed(ctx, s.pool, telegramID)
}

func hasConfirmed(ctx context.Context, q rowQuerier, telegramID int64) (has bool, err error) {
	err = q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM payments WHERE telegram_id = $1 AND status = 'confirmed')`, telegramID).Scan(&has)
	return has, err
}

// checkPromoLocked блокирует строку кода (FOR UPDATE) и заново проверяет его (promo.Check).
// Нет кода — promo.ErrNotFound; отказ — sentinel из promo.
func checkPromoLocked(
	ctx context.Context, tx pgx.Tx, promoID, telegramID int64, place promo.Place, now time.Time,
) (model.Promo, error) {
	p, err := scanPromo(tx.QueryRow(ctx, `SELECT `+promoCols+` FROM promo_codes WHERE id = $1 FOR UPDATE`, promoID))
	if errors.Is(err, ErrNotFound) {
		return p, errors.Join(promo.ErrNotFound, err)
	}
	if err != nil {
		return p, err
	}
	used, already, err := promoUsage(ctx, tx, promoID, telegramID)
	if err != nil {
		return p, err
	}
	confirmed, err := hasConfirmed(ctx, tx, telegramID)
	if err != nil {
		return p, err
	}
	return p, promo.Check(p, used, telegramID, confirmed, already, place, now)
}

// ActivateDaysPromo под блокировкой кода заново проверяет его (promo.Check, PlaceMenu),
// сдвигает базу pending-платежа юзера на те же дни (иначе подтверждение/отклонение затрёт бонус),
// вызывает apply (продление в панели) и записывает активацию. Ошибка apply откатывает всё.
// Нет кода — promo.ErrNotFound; отказ — sentinel из promo.
func (s *Store) ActivateDaysPromo(
	ctx context.Context, promoID, telegramID int64, now time.Time, apply func(p model.Promo) error,
) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		p, err := checkPromoLocked(ctx, tx, promoID, telegramID, promo.PlaceMenu, now)
		if err != nil {
			return err
		}
		// как в панели: later(срок, now) + N дней
		if _, err = tx.Exec(ctx,
			`UPDATE payments SET prev_expire_at = greatest(prev_expire_at, $2) + make_interval(days => $3)
			 WHERE telegram_id = $1 AND status = 'pending'`, telegramID, now, p.Value); err != nil {
			return err
		}
		if err = apply(p); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO promo_activations (promo_id, telegram_id) VALUES ($1, $2)`, promoID, telegramID)
		return err
	})
}
