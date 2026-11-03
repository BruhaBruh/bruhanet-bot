package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/model"
)

const userSummaryQuery = `
	SELECT u.telegram_id, u.username, u.created_at,
		count(p.id) FILTER (WHERE p.status = 'confirmed'),
		coalesce(sum(p.price) FILTER (WHERE p.status = 'confirmed'), 0)
	FROM users u LEFT JOIN payments p ON p.telegram_id = u.telegram_id`

func scanUserSummary(row pgx.Row) (u model.UserSummary, err error) {
	err = row.Scan(&u.TelegramID, &u.Username, &u.CreatedAt, &u.Confirmed, &u.ConfirmedSum)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return u, err
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SearchUsers ищет по подстроке username (без учёта регистра, ведущий @ срезается);
// q из одних цифр дополнительно совпадает по telegram_id. Пустой q — все. Новые сверху.
func (s *Store) SearchUsers(ctx context.Context, q string, limit int) ([]model.UserSummary, error) {
	q = strings.TrimPrefix(strings.TrimSpace(q), "@")
	var id *int64 // nil — по id не ищем (не число или не влезает в int64)
	if n, err := strconv.ParseInt(q, 10, 64); err == nil && !strings.ContainsAny(q, "+-") {
		id = &n
	}
	rows, err := s.pool.Query(ctx, userSummaryQuery+`
		WHERE $1 = '' OR u.username ILIKE '%'||$2||'%' ESCAPE '\' OR u.telegram_id = $3
		GROUP BY u.telegram_id ORDER BY u.created_at DESC, u.telegram_id DESC LIMIT $4`,
		q, likeEscaper.Replace(q), id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.UserSummary{}
	for rows.Next() {
		u, err := scanUserSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UserByID(ctx context.Context, id int64) (model.UserSummary, error) {
	return scanUserSummary(s.pool.QueryRow(ctx, userSummaryQuery+` WHERE u.telegram_id = $1 GROUP BY u.telegram_id`, id))
}

// UserByUsername — точное совпадение username без учёта регистра, ведущий @ срезается; нет — ErrNotFound.
func (s *Store) UserByUsername(ctx context.Context, username string) (model.UserSummary, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return model.UserSummary{}, ErrNotFound
	}
	return scanUserSummary(s.pool.QueryRow(ctx, userSummaryQuery+
		` WHERE lower(u.username) = lower($1) GROUP BY u.telegram_id ORDER BY u.created_at DESC LIMIT 1`, username))
}

func collectPayments(rows pgx.Rows) ([]model.Payment, error) {
	defer rows.Close()
	out := []model.Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PaymentsByUser(ctx context.Context, id int64) ([]model.Payment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+paymentCols+` FROM payments WHERE telegram_id = $1 ORDER BY id DESC`, id)
	if err != nil {
		return nil, err
	}
	return collectPayments(rows)
}

// ListPayments — страница платежей (id DESC) и общее число под фильтр.
func (s *Store) ListPayments(ctx context.Context, f model.PaymentFilter) ([]model.Payment, int, error) {
	const where = ` WHERE ($1 = '' OR status = $1)
		AND ($2::timestamptz IS NULL OR created_at >= $2) AND ($3::timestamptz IS NULL OR created_at < $3)`
	from, to := nullTime(f.From), nullTime(f.To)
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM payments`+where, string(f.Status), from, to).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+paymentCols+` FROM payments`+where+` ORDER BY id DESC LIMIT $4 OFFSET $5`,
		string(f.Status), from, to, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	ps, err := collectPayments(rows)
	return ps, total, err
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Revenue — сумма price confirmed-платежей с resolved_at в [from, to).
func (s *Store) Revenue(ctx context.Context, from, to time.Time) (int, error) {
	var sum int
	err := s.pool.QueryRow(ctx,
		`SELECT coalesce(sum(price), 0) FROM payments WHERE status = 'confirmed' AND resolved_at >= $1 AND resolved_at < $2`,
		from, to).Scan(&sum)
	return sum, err
}
