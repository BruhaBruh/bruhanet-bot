package store

import "context"

// TextOverrides возвращает все переопределения текстов (key → body).
func (s *Store) TextOverrides(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, body FROM texts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, b string
		if err = rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		out[k] = b
	}
	return out, rows.Err()
}

func (s *Store) SetText(ctx context.Context, key, body string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO texts (key, body) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET body = EXCLUDED.body, updated_at = now()`, key, body)
	return err
}

// ResetText удаляет переопределение; отсутствие ключа — не ошибка.
func (s *Store) ResetText(ctx context.Context, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM texts WHERE key = $1`, key)
	return err
}

// SettingsRaw возвращает сырой JSON каждой сохранённой настройки.
func (s *Store) SettingsRaw(ctx context.Context) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var k string
		var v []byte
		if err = rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetSetting сохраняет JSON-значение настройки (невалидный JSON отвергает Postgres).
func (s *Store) SetSetting(ctx context.Context, key string, value []byte) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, string(value))
	return err
}
