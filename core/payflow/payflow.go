// Package payflow — решение по платежу, одно на бота и панель: даты, запись в панель и уведомления.
package payflow

import (
	"context"
	"errors"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

// Month — месяц подписки.
const Month = 30 * 24 * time.Hour

// События outbox.
const (
	EventResult      = "payment_result"
	EventAdminClosed = "payment_admin_closed"
)

// ErrPanel — панель не приняла новый срок; транзакция откатана, повтор считает ту же дату.
var ErrPanel = errors.New("payflow: panel update failed")

type Panel interface {
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
}

// Store — core/store: ошибки store.ErrNotFound и store.ErrNotPending проходят как есть.
type Store interface {
	ResolvePayment(ctx context.Context, id int64, status model.PaymentStatus, at time.Time,
		apply func(model.Payment) ([]model.OutboxMessage, error)) (model.Payment, error)
}

// Decision — что решил админ. AdminID не ноль — решение из панели: в той же транзакции кладётся
// EventAdminClosed, воркер закрывает им сообщение админа в боте (в боте его закрывает сам хендлер).
type Decision struct {
	ID      int64
	Approve bool
	At      time.Time
	Squad   uuid.UUID
	AdminID int64
}

// ExtendedExpire — срок после подтверждения: от прежнего срока или от at, если он в прошлом.
func ExtendedExpire(prev, at time.Time, months int) time.Time {
	if at.After(prev) {
		prev = at
	}
	return prev.Add(time.Duration(months) * Month)
}

// Resolve под блокировкой строки платежа записывает в панель абсолютную дату (подтверждение — ExtendedExpire,
// отклонение — возврат PrevExpireAt) и кладёт уведомления в outbox. Текст и клавиатуру юзеру рендерит воркер при доставке.
func Resolve(ctx context.Context, st Store, panel Panel, d Decision) (model.Payment, error) {
	status, key := model.PaymentDeclined, "payment.declined"
	if d.Approve {
		status, key = model.PaymentConfirmed, "payment.confirmed"
	}
	return st.ResolvePayment(ctx, d.ID, status, d.At, func(p model.Payment) ([]model.OutboxMessage, error) {
		expireAt := p.PrevExpireAt
		if d.Approve {
			expireAt = ExtendedExpire(p.PrevExpireAt, d.At, p.Month)
		}
		if _, err := panel.UpdateUser(ctx, p.TelegramID, p.Username, expireAt, d.Squad); err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		msgs := []model.OutboxMessage{{EventType: EventResult, ChatID: p.TelegramID, TextKey: key, ExpireAt: expireAt, ParseMode: "HTML"}}
		if d.AdminID != 0 {
			msgs = append(msgs, model.OutboxMessage{
				EventType: EventAdminClosed, ChatID: d.AdminID, MessageID: p.AdminMessageID,
				Confirmed: d.Approve, Months: p.Month, Price: p.Price,
			})
		}
		return msgs, nil
	})
}
