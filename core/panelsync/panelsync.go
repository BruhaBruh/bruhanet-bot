// Package panelsync копирует срок и статус подписки из панели Remnawave в таблицу users.
// Один код на cron бота и ручной запуск из админки.
package panelsync

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/model"
)

// ErrPanel — панель недоступна или вернула пустой список; данные не тронуты.
var ErrPanel = errors.New("panel sync: panel unavailable")

// Timeout — потолок одного прохода; меньше таймаута nginx на ручной запуск.
const Timeout = 50 * time.Second

type Panel interface {
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

// Store — подмножество core/store.
type Store interface {
	WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error
	ApplyPanelSync(ctx context.Context, users []model.PanelUser, started, finished time.Time) (int, error)
}

// Run сначала целиком читает панель и только потом пишет одной транзакцией: сбой посреди обхода
// ничего не перезаписывает. Параллельный запуск получает store.ErrSyncRunning.
func Run(ctx context.Context, p Panel, st Store, now func() time.Time) (model.PanelSync, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	var res model.PanelSync
	err := st.WithPanelSyncLock(ctx, func(ctx context.Context) error {
		started := now()
		users, err := collect(ctx, p)
		if err != nil {
			return err
		}
		res.FinishedAt = now()
		res.Total = len(users)
		res.Updated, err = st.ApplyPanelSync(ctx, users, started, res.FinishedAt)
		return err
	})
	return res, err
}

// collect — юзеры панели с Telegram ID, по первому аккаунту на ID (как GetUser).
// Пустой ответ считается сбоем: иначе он стёр бы статусы всем.
func collect(ctx context.Context, p Panel) ([]model.PanelUser, error) {
	users := []model.PanelUser{}
	seen := map[int64]struct{}{}
	for u, err := range p.Users(ctx) {
		if err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		tg, ok := u.TelegramId.Get()
		if !ok || tg == 0 {
			continue
		}
		id := int64(tg)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		name := u.Username
		if name == strconv.FormatInt(id, 10) { // бот отдаёт панели ID, когда username нет
			name = ""
		}
		users = append(users, model.PanelUser{
			TelegramID: id, Username: name, Status: string(u.Status), ExpireAt: u.ExpireAt, CreatedAt: u.CreatedAt,
		})
	}
	if len(users) == 0 {
		return nil, errors.Join(ErrPanel, errors.New("panel returned no users"))
	}
	return users, nil
}
