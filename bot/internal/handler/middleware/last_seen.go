package middleware

import (
	"context"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// LastSeenEvery — юзер пишется в БД не чаще этого интервала.
const LastSeenEvery = 5 * time.Minute

const (
	lastSeenTimeout = 2 * time.Second
	lastSeenMaxKeys = 100_000
)

// LastSeen после хендлера пишет имя и время визита юзера: не задерживает ответ, сбой только логируется.
// Считаются только сообщения в личке и нажатия кнопок: это то, что юзер сделал сам (не my_chat_member).
//
// ponytail: память процесса: рестарт даст по лишней записи на юзера, это дешевле отдельного ключа в Redis.
// Заблокировал и разблокировал бота внутри окна: blocked_bot_at снимется при следующей записи, позже окна.
func LastSeen(touch func(context.Context, model.UserSeen) error, every time.Duration, now func() time.Time) bot.Middleware {
	var (
		mu   sync.Mutex
		last = map[int64]time.Time{}
	)
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			next(ctx, b, u)

			from := actingUser(u)
			if from == nil || from.IsBot {
				return
			}
			at := now()
			mu.Lock()
			due := at.Sub(last[from.ID]) >= every
			if due {
				if len(last) >= lastSeenMaxKeys {
					clear(last) // ponytail: потолок памяти; цена — по лишней записи на юзера
				}
				last[from.ID] = at
			}
			mu.Unlock()
			if !due {
				return
			}

			ctx, cancel := context.WithTimeout(ctx, lastSeenTimeout)
			defer cancel()
			if err := touch(ctx, model.UserSeen{
				TelegramID: from.ID, Username: from.Username, FirstName: from.FirstName, LastName: from.LastName, At: at,
			}); err != nil {
				stlog.Warn(ctx, "failed to save last seen", "error", err)
				mu.Lock()
				delete(last, from.ID) // следующий апдейт повторит запись
				mu.Unlock()
			}
		}
	}
}

func actingUser(u *models.Update) *models.User {
	switch {
	case u.Message != nil && u.Message.Chat.Type == models.ChatTypePrivate:
		return u.Message.From
	case u.CallbackQuery != nil:
		return &u.CallbackQuery.From
	}
	return nil
}
