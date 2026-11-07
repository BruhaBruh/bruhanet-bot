package store

import (
	"context"
	"encoding/json"

	"github.com/bruhabruh/bruhanet/core/texts"
)

// KeyboardLayouts возвращает сохранённые раскладки клавиатур (id клавиатуры → раскладка). Проверку по реестру
// делает вызывающий: строка, не прошедшая её, не должна ломать остальные.
func (s *Store) KeyboardLayouts(ctx context.Context) (map[string]texts.Layout, error) {
	rows, err := s.pool.Query(ctx, `SELECT keyboard_id, rows, hidden FROM text_keyboards`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]texts.Layout{}
	for rows.Next() {
		var id string
		var r, h []byte
		if err = rows.Scan(&id, &r, &h); err != nil {
			return nil, err
		}
		var l texts.Layout
		if json.Unmarshal(r, &l.Rows) != nil || json.Unmarshal(h, &l.Hidden) != nil {
			continue // повреждённая строка: как будто её нет, бот на раскладке по умолчанию
		}
		out[id] = l
	}
	return out, rows.Err()
}

// SaveKeyboardLayout сохраняет раскладку клавиатуры id (upsert).
func (s *Store) SaveKeyboardLayout(ctx context.Context, id string, l texts.Layout) error {
	if l.Rows == nil {
		l.Rows = [][]string{}
	}
	if l.Hidden == nil {
		l.Hidden = []string{}
	}
	rows, err := json.Marshal(l.Rows)
	if err != nil {
		return err
	}
	hidden, err := json.Marshal(l.Hidden)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO text_keyboards (keyboard_id, rows, hidden) VALUES ($1, $2::jsonb, $3::jsonb)
		ON CONFLICT (keyboard_id) DO UPDATE SET rows = EXCLUDED.rows, hidden = EXCLUDED.hidden, updated_at = now()`,
		id, string(rows), string(hidden))
	return err
}

// DeleteKeyboardLayout сбрасывает раскладку на расположение из кода; отсутствие строки — не ошибка.
func (s *Store) DeleteKeyboardLayout(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM text_keyboards WHERE keyboard_id = $1`, id)
	return err
}
