package app

import (
	"log/slog"

	"github.com/robfig/cron/v3"
)

// cronLogger адаптирует slog под cron.Logger: паника задачи пишется как `panic recovered`
// (то же сообщение, что в middleware.Recovery — на нём строятся алерты), прочее штатное — Debug.
func cronLogger(logger *slog.Logger) cron.Logger { return cronLog{logger} }

type cronLog struct{ l *slog.Logger }

func (c cronLog) Info(msg string, kv ...any) { c.l.Debug(msg, kv...) }

func (c cronLog) Error(err error, msg string, kv ...any) {
	if msg == "panic" {
		c.l.Error("panic recovered", append([]any{"component", "reminder", "error", err}, kv...)...)
		return
	}
	c.l.Error(msg, append([]any{"error", err}, kv...)...)
}
