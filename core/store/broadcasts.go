package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
)

const broadcastCols = `id, status, text, COALESCE(photo, ''), audience, audience_days, silent, buttons, scheduled_at,
	total, sent, failed, created_at, updated_at, finished_at`

func scanBroadcast(r pgx.Row) (model.Broadcast, error) {
	var (
		b       model.Broadcast
		buttons []byte
	)
	err := r.Scan(&b.ID, &b.Status, &b.Text, &b.Photo, &b.Audience, &b.AudienceDays, &b.Silent, &buttons, &b.ScheduledAt,
		&b.Total, &b.Sent, &b.Failed, &b.CreatedAt, &b.UpdatedAt, &b.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return b, err
	}
	b.Buttons = []broadcast.Button{}
	return b, json.Unmarshal(buttons, &b.Buttons)
}

func buttonsJSON(bs []broadcast.Button) ([]byte, error) {
	if bs == nil {
		bs = []broadcast.Button{}
	}
	return json.Marshal(bs)
}

// recipients — telegram ID получателей на момент now: не заблокировавшие бота и не заблокированные админом.
func recipients(ctx context.Context, q querier, audience string, days *int, now time.Time) ([]int64, error) {
	where, args := audienceWhere(audience, days, now)
	rows, err := q.Query(ctx, `SELECT telegram_id FROM users WHERE `+where+` ORDER BY telegram_id`, args...)
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

// audienceWhere — условие по users; $1 — now (и $2 — число дней у expiring).
func audienceWhere(audience string, days *int, now time.Time) (string, []any) {
	const base = `blocked_bot_at IS NULL AND blocked_at IS NULL`
	switch audience {
	case broadcast.AudienceActive:
		return base + ` AND expire_at > $1`, []any{now}
	case broadcast.AudienceNoSubscription:
		return base + ` AND (expire_at IS NULL OR expire_at <= $1)`, []any{now}
	case broadcast.AudienceExpiring:
		return base + ` AND expire_at > $1 AND expire_at <= $1 + make_interval(days => $2)`, []any{now, *days}
	case broadcast.AudienceTrial:
		return base + ` AND ` + trialOpen("users"), nil
	}
	return base, nil
}

// AudienceCounts — сколько получателей в каждой аудитории на момент now; expiring считается за days дней (0 — не считать).
func (s *Store) AudienceCounts(ctx context.Context, now time.Time, days int) (model.AudienceCounts, error) {
	var c model.AudienceCounts
	err := s.pool.QueryRow(ctx, `SELECT count(*),
			count(*) FILTER (WHERE expire_at > $1),
			count(*) FILTER (WHERE expire_at IS NULL OR expire_at <= $1),
			count(*) FILTER (WHERE $2 > 0 AND expire_at > $1 AND expire_at <= $1 + make_interval(days => $2)),
			count(*) FILTER (WHERE `+trialOpen("users")+`)
		FROM users WHERE blocked_bot_at IS NULL AND blocked_at IS NULL`, now, days).
		Scan(&c.All, &c.Active, &c.NoSubscription, &c.Expiring, &c.Trial)
	return c, err
}

// startLocked переводит scheduled-рассылку (строка заблокирована вызывающим) в sending: считает получателей на now и
// в той же транзакции кладёт им строки outbox, итог админу — последним. Условие status = 'scheduled' не даёт запустить
// одну рассылку дважды.
func startLocked(ctx context.Context, tx pgx.Tx, b model.Broadcast, adminID int64, now time.Time) (model.Broadcast, error) {
	if err := broadcast.ValidateAudience(b.Audience, b.AudienceDays); err != nil {
		return b, err
	}
	ids, err := recipients(ctx, tx, b.Audience, b.AudienceDays, now)
	if err != nil {
		return b, err
	}
	b, err = scanBroadcast(tx.QueryRow(ctx, `UPDATE broadcasts SET status = 'sending', total = $2, updated_at = now()
		WHERE id = $1 AND status = 'scheduled' RETURNING `+broadcastCols, b.ID, len(ids)))
	if err != nil {
		return b, err
	}
	id := b.ID.String()
	kb := broadcast.Keyboard(b.Buttons)
	// ponytail: по INSERT на получателя; на десятках тысяч — CopyFrom.
	for _, chat := range ids {
		if err = enqueue(ctx, tx, model.OutboxMessage{
			EventType: "broadcast", ChatID: chat, Text: b.Text, Photo: b.Photo, Template: texts.HasVars(b.Text),
			ParseMode: string(models.ParseModeHTML), Broadcast: id, Keyboard: kb, Silent: b.Silent,
		}); err != nil {
			return b, err
		}
	}
	return b, enqueue(ctx, tx, model.OutboxMessage{
		EventType: "broadcast_summary", ChatID: adminID, ParseMode: string(models.ParseModeHTML), Broadcast: id,
	})
}

// SendBroadcast ставит рассылку в работу одной транзакцией. Без ScheduledAt она сразу уходит получателям (аудитория
// считается на now), с ScheduledAt остаётся scheduled до ClaimDue. DraftID задан — отправляется этот черновик, иначе
// создаётся новая строка. Ключ уже был — возвращает существующую рассылку и false, outbox не трогает (двойной клик).
func (s *Store) SendBroadcast(ctx context.Context, d model.BroadcastDraft, adminID int64, now time.Time) (model.Broadcast, bool, error) {
	if err := broadcast.ValidateAudience(d.Audience, d.AudienceDays); err != nil {
		return model.Broadcast{}, false, err
	}
	buttons, err := buttonsJSON(d.Buttons)
	if err != nil {
		return model.Broadcast{}, false, err
	}
	var (
		b       model.Broadcast
		created bool
	)
	err = s.tx(ctx, func(tx pgx.Tx) error {
		created = false
		var err error
		if b, err = scanBroadcast(tx.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE idempotency_key = $1`, d.IdempotencyKey)); !errors.Is(err, ErrNotFound) {
			return err
		}
		if d.DraftID != uuid.Nil {
			b, err = scanBroadcast(tx.QueryRow(ctx, `UPDATE broadcasts SET idempotency_key = $2, status = 'scheduled', text = $3,
				photo = NULLIF($4, ''), audience = $5, audience_days = $6, silent = $7, buttons = $8, scheduled_at = $9, updated_at = now()
				WHERE id = $1 AND status = 'draft' RETURNING `+broadcastCols,
				d.DraftID, d.IdempotencyKey, d.Text, d.Photo, d.Audience, d.AudienceDays, d.Silent, buttons, d.ScheduledAt))
			if errors.Is(err, ErrNotFound) {
				// черновика нет или он уже ушёл: тот же ключ мог взять параллельный запрос
				if b, err = scanBroadcast(tx.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE idempotency_key = $1`, d.IdempotencyKey)); !errors.Is(err, ErrNotFound) {
					return err
				}
				return lockedOrMissing(ctx, tx, d.DraftID)
			}
		} else {
			b, err = scanBroadcast(tx.QueryRow(ctx, `INSERT INTO broadcasts (id, idempotency_key, status, text, photo, audience, audience_days,
				silent, buttons, scheduled_at, total) VALUES ($1, $2, 'scheduled', $3, NULLIF($4, ''), $5, $6, $7, $8, $9, 0)
				ON CONFLICT (idempotency_key) DO NOTHING RETURNING `+broadcastCols,
				uuid.New(), d.IdempotencyKey, d.Text, d.Photo, d.Audience, d.AudienceDays, d.Silent, buttons, d.ScheduledAt))
			if errors.Is(err, ErrNotFound) { // параллельный запрос с тем же ключом успел раньше
				b, err = scanBroadcast(tx.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE idempotency_key = $1`, d.IdempotencyKey))
				return err
			}
		}
		if err != nil {
			return err
		}
		created = true
		if d.ScheduledAt != nil {
			return nil
		}
		b, err = startLocked(ctx, tx, b, adminID, now)
		return err
	})
	return b, created, err
}

// lockedOrMissing — рассылка есть, но не черновик (ErrBroadcastLocked), либо её нет (ErrNotFound).
func lockedOrMissing(ctx context.Context, q querier, id uuid.UUID) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM broadcasts WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrBroadcastLocked
	}
	return ErrNotFound
}

// SaveDraft создаёт черновик с заданным id или обновляет существующий (повтор автосохранения идемпотентен).
// Строка не черновик — ErrBroadcastLocked. Содержимое не проверяется: черновик может быть неполным.
func (s *Store) SaveDraft(ctx context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error) {
	buttons, err := buttonsJSON(d.Buttons)
	if err != nil {
		return model.Broadcast{}, err
	}
	b, err := scanBroadcast(s.pool.QueryRow(ctx, `INSERT INTO broadcasts (id, status, text, photo, audience, audience_days, silent, buttons, scheduled_at, total)
		VALUES ($1, 'draft', $2, NULLIF($3, ''), $4, $5, $6, $7, $8, 0)
		ON CONFLICT (id) DO UPDATE SET text = EXCLUDED.text, photo = EXCLUDED.photo, audience = EXCLUDED.audience,
			audience_days = EXCLUDED.audience_days, silent = EXCLUDED.silent, buttons = EXCLUDED.buttons,
			scheduled_at = EXCLUDED.scheduled_at, updated_at = now()
		WHERE broadcasts.status = 'draft' RETURNING `+broadcastCols,
		id, d.Text, d.Photo, d.Audience, d.AudienceDays, d.Silent, buttons, d.ScheduledAt))
	if errors.Is(err, ErrNotFound) {
		err = ErrBroadcastLocked
	}
	return b, err
}

// UpdateScheduled меняет запланированную рассылку до старта. Уже стартовала или отменена — ErrBroadcastLocked
// (UPDATE ждёт блокировку строки у ClaimDue и перепроверяет статус).
func (s *Store) UpdateScheduled(ctx context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error) {
	if d.ScheduledAt == nil {
		return model.Broadcast{}, errors.New("scheduled broadcast needs a time")
	}
	if err := broadcast.ValidateAudience(d.Audience, d.AudienceDays); err != nil {
		return model.Broadcast{}, err
	}
	buttons, err := buttonsJSON(d.Buttons)
	if err != nil {
		return model.Broadcast{}, err
	}
	b, err := scanBroadcast(s.pool.QueryRow(ctx, `UPDATE broadcasts SET text = $2, photo = NULLIF($3, ''), audience = $4, audience_days = $5,
		silent = $6, buttons = $7, scheduled_at = $8, updated_at = now()
		WHERE id = $1 AND status = 'scheduled' RETURNING `+broadcastCols,
		id, d.Text, d.Photo, d.Audience, d.AudienceDays, d.Silent, buttons, d.ScheduledAt))
	if errors.Is(err, ErrNotFound) {
		err = lockedOrMissing(ctx, s.pool, id)
	}
	return b, err
}

// CancelBroadcast отменяет черновик или запланированную рассылку; стартовавшую — ErrBroadcastLocked. Повтор на
// уже отменённой возвращает её без ошибки.
func (s *Store) CancelBroadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error) {
	b, err := scanBroadcast(s.pool.QueryRow(ctx, `UPDATE broadcasts SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND status IN ('draft', 'scheduled') RETURNING `+broadcastCols, id))
	if errors.Is(err, ErrNotFound) {
		if b, err = s.Broadcast(ctx, id); err == nil && b.Status != broadcast.StatusCancelled {
			err = ErrBroadcastLocked
		}
	}
	return b, err
}

// maxClaimFails — сколько раз подряд не удавшийся запуск рассылки терпим, прежде чем отменить её.
const maxClaimFails = 3

// ClaimDue запускает запланированные рассылки, время которых наступило (scheduled_at <= now). Каждая — в своей
// транзакции: FOR UPDATE SKIP LOCKED + переход scheduled → sending и строки outbox в одном коммите, поэтому
// перезапуск или второй воркер не стартуют одну рассылку дважды. Возвращает число запущенных.
// Сбой запуска одной рассылки не держит очередь: она пропускается в этом проходе, остальные стартуют; после
// maxClaimFails сбоев подряд (счётчик в памяти) рассылка отменяется (cancelled), иначе ей вечно бы пытались.
// Ошибки отдаются вместе (errors.Join); отмена контекста прерывает проход без учёта сбоя.
func (s *Store) ClaimDue(ctx context.Context, adminID int64, now time.Time) (int, error) {
	n := 0
	var errs []error
	skip := []uuid.UUID{}
	for {
		var claimed uuid.UUID
		err := s.tx(ctx, func(tx pgx.Tx) error {
			claimed = uuid.Nil
			b, err := scanBroadcast(tx.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts
				WHERE status = 'scheduled' AND scheduled_at <= $1 AND id <> ALL($2) ORDER BY scheduled_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`,
				now, skip))
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			claimed = b.ID
			_, err = startLocked(ctx, tx, b, adminID, now)
			return err
		})
		switch {
		case err == nil && claimed == uuid.Nil:
			return n, errors.Join(errs...)
		case err == nil:
			n++
			s.claimDone(claimed)
		case claimed == uuid.Nil || ctx.Err() != nil:
			return n, errors.Join(append(errs, err)...)
		default:
			skip = append(skip, claimed)
			errs = append(errs, err)
			if s.claimFailed(claimed) >= maxClaimFails {
				if cerr := s.cancelPoisoned(ctx, claimed); cerr != nil {
					errs = append(errs, cerr)
				} else {
					errs = append(errs, fmt.Errorf("broadcast %s cancelled after %d failed starts", claimed, maxClaimFails))
				}
			}
		}
	}
}

func (s *Store) claimDone(id uuid.UUID) {
	s.claimMu.Lock()
	delete(s.claimFails, id)
	s.claimMu.Unlock()
}

func (s *Store) claimFailed(id uuid.UUID) int {
	s.claimMu.Lock()
	defer s.claimMu.Unlock()
	if s.claimFails == nil {
		s.claimFails = map[uuid.UUID]int{}
	}
	s.claimFails[id]++
	return s.claimFails[id]
}

func (s *Store) cancelPoisoned(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE broadcasts SET status = 'cancelled', updated_at = now() WHERE id = $1 AND status = 'scheduled'`, id)
	if err == nil {
		s.claimDone(id)
	}
	return err
}

// Broadcast — одна рассылка; нет — ErrNotFound.
func (s *Store) Broadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error) {
	return scanBroadcast(s.pool.QueryRow(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE id = $1`, id))
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
	return scanBroadcast(s.pool.QueryRow(ctx, `UPDATE broadcasts SET finished_at = COALESCE(finished_at, now()),
		status = CASE WHEN status = 'sending' THEN 'sent' ELSE status END
		WHERE id = $1 RETURNING `+broadcastCols, id))
}

// ListBroadcasts — страница рассылок и общее число. Новые первыми; только запланированные — по времени старта.
func (s *Store) ListBroadcasts(ctx context.Context, f model.BroadcastFilter) ([]model.Broadcast, int, error) {
	where, args := []string{"true"}, []any{}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if len(f.Statuses) > 0 {
		where = append(where, "status = ANY("+arg(f.Statuses)+")")
	}
	if !f.From.IsZero() {
		where = append(where, "created_at >= "+arg(f.From))
	}
	if !f.To.IsZero() {
		where = append(where, "created_at < "+arg(f.To))
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM broadcasts WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := `created_at DESC, id`
	if len(f.Statuses) == 1 && f.Statuses[0] == broadcast.StatusScheduled {
		order = `scheduled_at, id`
	}
	rows, err := s.pool.Query(ctx, `SELECT `+broadcastCols+` FROM broadcasts WHERE `+cond+` ORDER BY `+order+
		` LIMIT `+arg(f.Limit)+` OFFSET `+arg(f.Offset), args...)
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
