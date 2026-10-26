package middleware

import (
	"context"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Track учитывает хендлер в wg и отвязывает его ctx от отмены: остановка бота
// не должна рвать уже запущенный хендлер, его дожидаются через wg.
func Track(wg *sync.WaitGroup) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, u *models.Update) {
			wg.Add(1)
			defer wg.Done()
			next(context.WithoutCancel(ctx), b, u)
		}
	}
}
