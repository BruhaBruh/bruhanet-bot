// Package outbox доставляет сообщения из Postgres-очереди в Telegram.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	batch       = 25
	interval    = 40 * time.Millisecond // между отправками (лимит Telegram ~30/с)
	idle        = time.Second
	maxAttempts = 10

	legacySummary = "Рассылка завершена"
)

type Queue interface {
	OutboxBatch(ctx context.Context, limit int) ([]model.OutboxItem, error)
	DeleteOutbox(ctx context.Context, id int64) error
	BroadcastDelivered(ctx context.Context, id uuid.UUID, ok bool) error
	FinishBroadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error)
	DirectDelivered(ctx context.Context, id int64, ok bool) error
	MarkBotBlocked(ctx context.Context, telegramID int64) error
	AddBroadcastReceived(ctx context.Context, telegramID int64) error
}

// Notice — текст и клавиатура уведомления о платеже (routes.PaymentNotice).
type Notice func(ctx context.Context, chatID int64, key string, expireAt time.Time, days, devices int) (string, *models.InlineKeyboardMarkup)

// Render подставляет переменные в текст для получателя (content.Provider): читает не больше строки users.
type Render func(ctx context.Context, chatID int64, tpl string) string

type Worker struct {
	q      Queue
	b      *bot.Bot
	render Render
	notice Notice

	interval, idle time.Duration
	backoff        func(attempt int) time.Duration

	attempts map[int64]int
}

func New(q Queue, b *bot.Bot, render Render, notice Notice) *Worker {
	return &Worker{
		q: q, b: b, render: render, notice: notice, interval: interval, idle: idle,
		backoff:  func(n int) time.Duration { return min(time.Duration(1<<n)*time.Second, time.Minute) },
		attempts: map[int64]int{},
	}
}

// Run доставляет очередь до отмены ctx.
func (w *Worker) Run(ctx context.Context) {
	for ctx.Err() == nil {
		items, err := w.q.OutboxBatch(ctx, batch)
		if err != nil {
			if ctx.Err() == nil {
				stlog.Error(ctx, "outbox batch failed", "error", err)
			}
			sleep(ctx, w.idle)
			continue
		}
		if len(items) == 0 {
			sleep(ctx, w.idle)
			continue
		}
		for _, it := range items {
			if !w.process(ctx, it) {
				break
			}
			sleep(ctx, w.interval)
		}
	}
}

// process возвращает false, если пачку нужно прервать (строка остаётся первой).
func (w *Worker) process(ctx context.Context, it model.OutboxItem) bool {
	for ctx.Err() == nil {
		err := w.send(ctx, it)
		var tm *bot.TooManyRequestsError
		switch {
		case err == nil:
			observability.OutboxMessages.WithLabelValues(it.EventType, "sent").Inc()
			return w.remove(ctx, it, true)
		case errors.Is(err, bot.ErrorForbidden), errors.Is(err, bot.ErrorBadRequest):
			if errors.Is(err, bot.ErrorForbidden) {
				// Повтор идемпотентен: остаётся время первой блокировки.
				if merr := w.q.MarkBotBlocked(context.WithoutCancel(ctx), it.ChatID); merr != nil {
					stlog.Error(ctx, "failed to mark bot blocked", "error", merr)
				}
			}
			stlog.Warn(ctx, "outbox message rejected", "event_type", it.EventType, "error", err)
			observability.OutboxMessages.WithLabelValues(it.EventType, "dropped").Inc()
			return w.remove(ctx, it, false)
		case errors.As(err, &tm):
			observability.OutboxMessages.WithLabelValues(it.EventType, "retry").Inc()
			sleep(ctx, time.Duration(max(tm.RetryAfter, 1))*time.Second)
			continue
		}
		if ctx.Err() != nil {
			return false
		}
		w.attempts[it.ID]++
		n := w.attempts[it.ID]
		if n >= maxAttempts {
			stlog.Error(ctx, "outbox message dropped", "event_type", it.EventType, "attempts", n, "error", err)
			observability.OutboxMessages.WithLabelValues(it.EventType, "dropped").Inc()
			return w.remove(ctx, it, false)
		}
		// ponytail: head-of-line — один чат с 5xx держит очередь ~5 мин до дропа; упрёмся — слать по чатам независимо.
		stlog.Warn(ctx, "outbox send failed", "event_type", it.EventType, "attempt", n, "error", err)
		observability.OutboxMessages.WithLabelValues(it.EventType, "retry").Inc()
		sleep(ctx, w.backoff(n))
		return false
	}
	return false
}

// remove удаляет строку и только после этого учитывает исход (при сбое удаления строка уйдёт повторно).
// Удаление идёт без отмены: иначе отмена между отправкой и удалением даёт гарантированный дубль.
func (w *Worker) remove(ctx context.Context, it model.OutboxItem, ok bool) bool {
	delete(w.attempts, it.ID)
	if err := w.q.DeleteOutbox(context.WithoutCancel(ctx), it.ID); err != nil {
		// at-least-once: строка отправится повторно
		stlog.Error(ctx, "outbox delete failed", "error", err)
		sleep(ctx, w.idle)
		return false
	}
	w.count(context.WithoutCancel(ctx), it, ok)
	return true
}

func (w *Worker) send(ctx context.Context, it model.OutboxItem) error {
	if it.EventType == payflow.EventAdminClosed {
		return w.closeAdminMessage(ctx, it)
	}
	if it.TextKey != "" {
		it.Text, it.Keyboard = w.notice(ctx, it.ChatID, it.TextKey, it.ExpireAt, it.Days, it.Devices)
	}
	if it.Template {
		it.Text = w.render(ctx, it.ChatID, it.Text)
	}
	switch {
	case it.EventType == "broadcast_summary":
		text, err := w.summary(ctx, it)
		if err != nil {
			return err
		}
		_, err = w.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: it.ChatID, ParseMode: models.ParseMode(it.ParseMode), Text: text,
		})
		return err
	case it.MessageID != 0:
		_, err := w.b.CopyMessage(ctx, &bot.CopyMessageParams{
			ChatID: it.ChatID, FromChatID: it.FromChatID, MessageID: it.MessageID,
		})
		return err
	case it.Photo != "":
		p := &bot.SendPhotoParams{
			ChatID: it.ChatID, Photo: &models.InputFileString{Data: it.Photo},
			Caption: it.Text, ParseMode: models.ParseMode(it.ParseMode), DisableNotification: it.Silent,
		}
		if it.Keyboard != nil { // nil-указатель в интерфейсе ReplyMarkup не годится
			p.ReplyMarkup = it.Keyboard
		}
		_, err := w.b.SendPhoto(ctx, p)
		return err
	default:
		p := &bot.SendMessageParams{ChatID: it.ChatID, Text: it.Text, ParseMode: models.ParseMode(it.ParseMode), DisableNotification: it.Silent}
		if it.Keyboard != nil {
			p.ReplyMarkup = it.Keyboard
		}
		_, err := w.b.SendMessage(ctx, p)
		return err
	}
}

// closeAdminMessage — платёж решён в панели: у сообщения админа в боте тот же исход, что после кнопки. Платёж уже
// закоммичен, сбой здесь его не отменяет. BadRequest на кнопках (их уже нет или ещё нет) реакцию не останавливает.
func (w *Worker) closeAdminMessage(ctx context.Context, it model.OutboxItem) error {
	if it.MessageID == 0 {
		return nil
	}
	_, err := w.b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID: it.ChatID, MessageID: it.MessageID,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}},
	})
	if err != nil && !errors.Is(err, bot.ErrorBadRequest) {
		return err
	}
	emoji := "👎"
	if it.Confirmed {
		emoji = "👍"
	}
	_, err = w.b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID: it.ChatID, MessageID: it.MessageID,
		Reaction: []models.ReactionType{{
			Type:              models.ReactionTypeTypeEmoji,
			ReactionTypeEmoji: &models.ReactionTypeEmoji{Type: models.ReactionTypeTypeEmoji, Emoji: emoji},
		}},
		IsBig: bot.True(),
	})
	return err
}

// summary — итог рассылки из счётчиков в БД; рассылки /a (нет в таблице) — заглушка.
func (w *Worker) summary(ctx context.Context, it model.OutboxItem) (string, error) {
	id, perr := uuid.Parse(it.Broadcast)
	if perr != nil { // строка /a без uuid
		return legacySummary, nil //nolint:nilerr // не ошибка доставки
	}
	b, err := w.q.FinishBroadcast(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		// ponytail: строки /a от прошлого деплоя; счётчиков у них нет — уходит вместе с /a.
		return legacySummary, nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Разослано: <b>%d</b>, не доставлено: <b>%d</b>", b.Sent, b.Failed), nil
}

// count учитывает исход доставки (рассылка — счётчики, личное — статус) после удаления строки.
// Сбой записи теряет одну отметку, но не дублирует отправку.
func (w *Worker) count(ctx context.Context, it model.OutboxItem, ok bool) {
	var err error
	switch it.EventType {
	case payflow.EventAdminClosed:
		// решение из панели: у api своего счётчика нет, бизнес-метрики платежей живут в боте
		stage := "declined"
		if it.Confirmed {
			stage = "confirmed"
			observability.PaymentsConfirmedRubles.WithLabelValues(strconv.Itoa(it.Months)).Add(float64(it.Price))
		}
		observability.Payments.WithLabelValues(stage, strconv.Itoa(it.Months)).Inc()
		return
	case "broadcast_summary":
		observability.Broadcasts.Inc() // одна рассылка = один итог (и /a-строки тоже)
		return
	case "broadcast":
		result := "failed"
		if ok {
			result = "sent"
		}
		observability.BroadcastMessages.WithLabelValues(result).Inc()
		if ok {
			// count идёт раз на строку outbox, после её удаления: повторная отправка не удваивает счётчик.
			// Падение между удалением и этой записью теряет одну отметку, как и у счётчиков рассылки.
			if rerr := w.q.AddBroadcastReceived(ctx, it.ChatID); rerr != nil {
				stlog.Error(ctx, "broadcast receipt not recorded", "error", rerr)
			}
		}
		id, perr := uuid.Parse(it.Broadcast)
		if perr != nil {
			return // строка /a без uuid
		}
		if err = w.q.BroadcastDelivered(ctx, id, ok); errors.Is(err, store.ErrNotFound) {
			return // рассылка /a: в таблице её нет
		}
	case "direct":
		err = w.q.DirectDelivered(ctx, it.DirectID, ok)
	default:
		return
	}
	if err != nil {
		stlog.Error(ctx, "outbox delivery not recorded", "event_type", it.EventType, "error", err)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
