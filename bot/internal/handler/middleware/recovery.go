package middleware

import (
	"context"
	"runtime/debug"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// Recovery ставится в цепочку дважды: первым (страховка для самих middleware)
// и перед FSM (паника хендлера, чтобы Logger и AutoAnswer отработали).
func Recovery(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, u *models.Update) {
		defer func() {
			if rec := recover(); rec != nil {
				UpdateInfoFrom(ctx).MarkPanicked()
				stlog.Error(ctx, "panic recovered", "panic", rec, "stack", string(debug.Stack()))
			}
		}()
		next(ctx, b, u)
	}
}
