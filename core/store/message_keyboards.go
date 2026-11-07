package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/bruhabruh/bruhanet/core/texts"
)

// MessageKeyboards возвращает сохранённые клавиатуры сообщений (ключ текста → клавиатура). Проверку по реестру
// делает вызывающий: повреждённая строка пропускается, остальные не страдают.
func (s *Store) MessageKeyboards(ctx context.Context) (map[string]texts.MsgLayout, error) {
	rows, err := s.pool.Query(ctx, `SELECT text_key, rows FROM message_keyboards`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]texts.MsgLayout{}
	for rows.Next() {
		var key string
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var l texts.MsgLayout
		if json.Unmarshal(raw, &l.Rows) != nil {
			continue
		}
		out[key] = l
	}
	return out, rows.Err()
}

// SaveMessageKeyboard сохраняет клавиатуру сообщения key (upsert).
func (s *Store) SaveMessageKeyboard(ctx context.Context, key string, l texts.MsgLayout) error {
	raw, err := json.Marshal(l.Rows)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO message_keyboards (text_key, rows) VALUES ($1, $2::jsonb)
		ON CONFLICT (text_key) DO UPDATE SET rows = EXCLUDED.rows, updated_at = now()`, key, string(raw))
	return err
}

// DeleteMessageKeyboard сбрасывает клавиатуру сообщения на код; отсутствие строки — не ошибка.
func (s *Store) DeleteMessageKeyboard(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM message_keyboards WHERE text_key = $1`, key)
	return err
}

// MigrateLegacyKeyboards переносит общие раскладки text_keyboards в клавиатуры сообщений (message_keyboards) и
// удаляет перенесённые строки — в одной транзакции, поэтому повтор ничего не делает. Сообщение, у которого уже есть
// своя клавиатура, не затирается. Возвращает число перенесённых сообщений.
func (s *Store) MigrateLegacyKeyboards(ctx context.Context) (int, error) {
	n := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		n = 0
		rows, err := tx.Query(ctx, `SELECT keyboard_id, rows, hidden FROM text_keyboards FOR UPDATE`)
		if err != nil {
			return err
		}
		legacy := map[string]texts.Layout{}
		var ids []string // только прочитанные и разобранные строки: остальное (битый JSON, вставленное позже) не трогаем
		for rows.Next() {
			var id string
			var r, h []byte
			if err = rows.Scan(&id, &r, &h); err != nil {
				rows.Close()
				return err
			}
			var l texts.Layout
			if json.Unmarshal(r, &l.Rows) == nil && json.Unmarshal(h, &l.Hidden) == nil {
				legacy[id] = l
				ids = append(ids, id)
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		invalid := false
		for _, key := range texts.MessagesWithKeyboard() {
			l, ok := texts.LegacyToLayout(key, legacy)
			if !ok {
				continue
			}
			// клавиатура, которую бот всё равно отбросит, не переносится; старые строки остаются для разбора
			if texts.ValidateMessageLayout(key, l, nil, false) != nil {
				invalid = true
				continue
			}
			raw, err := json.Marshal(l.Rows)
			if err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `INSERT INTO message_keyboards (text_key, rows) VALUES ($1, $2::jsonb) ON CONFLICT (text_key) DO NOTHING`, key, string(raw))
			if err != nil {
				return err
			}
			n += int(tag.RowsAffected())
		}
		if invalid || len(ids) == 0 {
			return nil
		}
		_, err = tx.Exec(ctx, `DELETE FROM text_keyboards WHERE keyboard_id = ANY($1)`, ids)
		return err
	})
	return n, err
}
