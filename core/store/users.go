package store

import (
	"context"

	"github.com/bruhabruh/bruhanet/core/model"
)

// TouchUser заводит юзера при первом апдейте и обновляет имя и время визита;
// написавший снова юзер точно не блокирует бота, поэтому blocked_bot_at снимается.
func (s *Store) TouchUser(ctx context.Context, u model.UserSeen) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (telegram_id, username, first_name, last_name, last_seen_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5)
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username, first_name = EXCLUDED.first_name, last_name = EXCLUDED.last_name,
			last_seen_at = EXCLUDED.last_seen_at, blocked_bot_at = NULL`,
		u.TelegramID, u.Username, u.FirstName, u.LastName, u.At)
	return err
}
