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

// EventGrant — уведомление юзеру о днях, выданных из панели.
const EventGrant = "grant_days"

// ErrNotRestored — после сбоя коммита прежний срок в панели вернуть не удалось: расхождение, дочинит синхронизация.
var ErrNotRestored = errors.New("payflow: panel expire not restored")

// ErrGrantDays — дни вне 1..MaxGrantDays.
var ErrGrantDays = errors.New("payflow: grant days out of range")

// MaxGrantDays — потолок одной выдачи.
const MaxGrantDays = 365

// GrantPanel — панель для выдачи дней: создать юзера, прочитать и записать срок.
type GrantPanel interface {
	Panel
	CreateUser(ctx context.Context, telegramID int64, username string, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
}

// GrantStore — core/store: ErrNotFound (нет юзера) проходит как есть.
type GrantStore interface {
	GrantDays(ctx context.Context, telegramID, adminID int64, days int, now time.Time,
		apply func() ([]model.OutboxMessage, error)) error
}

// GrantRequest — выдача дней админом.
type GrantRequest struct {
	TelegramID int64
	Username   string
	Days       int
	AdminID    int64
	At         time.Time
	Squad      uuid.UUID
}

// GrantDays продлевает юзера на Days дней от later(срок в панели, At) — абсолютной датой — и возвращает её.
// Всё под транзакцией store.GrantDays (сдвиг pending-платежа, строка grants, уведомление в outbox); ошибка панели
// откатывает транзакцию (ErrPanel). Если панель уже продлена, а коммит не прошёл, прежний срок возвращается —
// только пока срок в панели всё ещё наш (иначе его успел поменять админ — не затираем).
func GrantDays(ctx context.Context, st GrantStore, panel GrantPanel, r GrantRequest) (time.Time, error) {
	if r.Days < 1 || r.Days > MaxGrantDays {
		return time.Time{}, ErrGrantDays
	}
	var oldExpire, newExpire time.Time
	err := st.GrantDays(ctx, r.TelegramID, r.AdminID, r.Days, r.At, func() ([]model.OutboxMessage, error) {
		u, err := panel.CreateUser(ctx, r.TelegramID, r.Username, r.Squad)
		if err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		next := u.ExpireAt
		if r.At.After(next) {
			next = r.At
		}
		next = next.Add(time.Duration(r.Days) * 24 * time.Hour)
		if _, err = panel.UpdateUser(ctx, r.TelegramID, r.Username, next, r.Squad); err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		oldExpire, newExpire = u.ExpireAt, next
		return []model.OutboxMessage{{
			EventType: EventGrant, ChatID: r.TelegramID, TextKey: "grant.days", Days: r.Days, ExpireAt: next, ParseMode: "HTML",
		}}, nil
	})
	if err != nil && !newExpire.IsZero() {
		// ponytail: GetUser→UpdateUser не атомарен; окно — миллисекунды после редкого сбоя коммита.
		cctx := context.WithoutCancel(ctx)
		cur, gerr := panel.GetUser(cctx, r.TelegramID)
		switch {
		case gerr != nil:
			err = errors.Join(err, ErrNotRestored, gerr)
		case cur.ExpireAt.Equal(newExpire):
			if _, uerr := panel.UpdateUser(cctx, r.TelegramID, r.Username, oldExpire, r.Squad); uerr != nil {
				err = errors.Join(err, ErrNotRestored, uerr)
			}
		}
	}
	return newExpire, err
}
