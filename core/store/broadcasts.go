package store

import (
	"context"
	"errors"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
)

const broadcastCols = `id, text, COALESCE(photo, ''), total, sent, failed, created_at, finished_at`

func scanBroadcast(r pgx.Row) (model.Broadcast, error) {
	var b model.Broadcast
	err := r.Scan(&b.ID, &b.Text, &b.Photo, &b.Total, &b.Sent, &b.Failed, &b.CreatedAt, &b.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return b, err
}

// CreateBroadcast в одной транзакции создаёт рассылку, строки outbox получателям и итог админу последним.
// Ключ уже был — возвращает существующую рассылку и false, outbox не трогает.
func (s *Store) CreateBroadcast(ctx context.Context, d model.BroadcastDraft, recipients []int64, adminID int64) (model.Broadcast, bool, error) {
	var (
		b       model.Broadcast
		created bool
	)
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		b, err = scanBroadcast(tx.QueryRow(ctx, `INSERT INTO broadcasts (id, idempotency_key, text, photo, total)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5)
			ON CONFLICT (idempotency_key) DO NOTHING
			RETURNING `+broadcastCols, uuid.New(), d.IdempotencyKey, d.Text, d.Photo, len(recipients)))
		if errors.Is(err, ErrNotFound) {
			b, err = scanBroadcast(tx.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE idempotency_key = $1`, d.IdempotencyKey))
			return err
		}
		if err != nil {
			return err
		}
		created = true
		id := b.ID.String()
		// ponytail: по INSERT на получателя; на десятках тысяч — CopyFrom.
		for _, chat := range recipients {
			if err = enqueue(ctx, tx, model.OutboxMessage{
				EventType: "broadcast", ChatID: chat, Text: d.Text, Photo: d.Photo, Template: texts.HasVars(d.Text),
				ParseMode: string(models.ParseModeHTML), Broadcast: id,
			}); err != nil {
				return err
			}
		}
		return enqueue(ctx, tx, model.OutboxMessage{
			EventType: "broadcast_summary", ChatID: adminID, ParseMode: string(models.ParseModeHTML), Broadcast: id,
		})
	})
	return b, created, err
}

// BroadcastDelivered учитывает исход доставки одному получателю; неизвестная рассылка — ErrNotFound.
func (s *Store) BroadcastDelivered(ctx context.Context, id uuid.UUID, ok bool) error {
	q := `UPDATE broadcasts SET failed = failed + 1 WHERE id = $1`
	if ok {
		q = `UPDATE broadcasts SET sent = sent + 1 WHERE id = $1`
	}
	return affected(s.pool.Exec(ctx, q, id))
}

// FinishBroadcast отмечает рассылку завершённой (первое завершение остаётся) и возвращает счётчики.
func (s *Store) FinishBroadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error) {
	return scanBroadcast(s.pool.QueryRow(ctx, `UPDATE broadcasts SET finished_at = COALESCE(finished_at, now())
		WHERE id = $1 RETURNING `+broadcastCols, id))
}

// ListBroadcasts — страница рассылок, новые первыми, и общее число.
func (s *Store) ListBroadcasts(ctx context.Context, limit, offset int) ([]model.Broadcast, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM broadcasts`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+broadcastCols+` FROM broadcasts
		ORDER BY created_at DESC, id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Broadcast{}
	for rows.Next() {
		b, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// CreateDirect записывает личное сообщение и строку outbox для него в одной транзакции.
func (s *Store) CreateDirect(ctx context.Context, telegramID int64, text, photo string) (int64, error) {
	var id int64
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO direct_messages (telegram_id, text, photo)
			VALUES ($1, $2, NULLIF($3, '')) RETURNING id`, telegramID, text, photo).Scan(&id); err != nil {
			return err
		}
		return enqueue(ctx, tx, model.OutboxMessage{
			EventType: "direct", ChatID: telegramID, Text: text, Photo: photo, Template: texts.HasVars(text),
			ParseMode: string(models.ParseModeHTML), DirectID: id,
		})
	})
	return id, err
}

// DirectDelivered ставит статус sent/failed; неизвестное сообщение — ErrNotFound.
func (s *Store) DirectDelivered(ctx context.Context, id int64, ok bool) error {
	status := model.DirectFailed
	if ok {
		status = model.DirectSent
	}
	return affected(s.pool.Exec(ctx, `UPDATE direct_messages SET status = $2 WHERE id = $1`, id, status))
}

// DirectMessages — личные сообщения юзеру, новые первыми.
func (s *Store) DirectMessages(ctx context.Context, telegramID int64) ([]model.DirectMessage, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, telegram_id, text, COALESCE(photo, ''), status, created_at
		FROM direct_messages WHERE telegram_id = $1 ORDER BY id DESC`, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DirectMessage{}
	for rows.Next() {
		var m model.DirectMessage
		if err = rows.Scan(&m.ID, &m.TelegramID, &m.Text, &m.Photo, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// affected превращает «ни одной строки не обновлено» в ErrNotFound.
func affected(tag pgconn.CommandTag, err error) error {
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
