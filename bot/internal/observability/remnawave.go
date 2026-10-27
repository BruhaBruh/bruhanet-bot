package observability

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

// Remnawave — подмножество клиента панели, которое используют хендлеры и reminder.
type Remnawave interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
	CreateUser(ctx context.Context, telegramID int64, username string, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

type instrumented struct{ rw Remnawave }

// InstrumentRemnawave оборачивает клиент спанами и метриками. ErrUserNotFound — ответ панели, не сбой.
func InstrumentRemnawave(rw Remnawave) Remnawave { return instrumented{rw} }

func call[T any](ctx context.Context, op string, f func(context.Context) (T, error)) (T, error) {
	if isProbe(ctx) {
		return f(ctx)
	}
	ctx, span := startSpan(ctx, "remnawave "+op)
	start := time.Now()
	v, err := f(ctx)
	failed := err
	if errors.Is(err, remnawave.ErrUserNotFound) {
		failed = nil
	}
	finish(span, "remnawave", op, start, failed)
	return v, err
}

func (i instrumented) GetUser(ctx context.Context, id int64) (*api.UserItemInfo, error) {
	return call(ctx, "GetUser", func(ctx context.Context) (*api.UserItemInfo, error) { return i.rw.GetUser(ctx, id) })
}

func (i instrumented) CreateUser(ctx context.Context, id int64, username string, squad uuid.UUID) (*api.UserItemInfo, error) {
	return call(ctx, "CreateUser", func(ctx context.Context) (*api.UserItemInfo, error) {
		return i.rw.CreateUser(ctx, id, username, squad)
	})
}

func (i instrumented) UpdateUser(ctx context.Context, id int64, username string, expireAt time.Time, squad uuid.UUID) (*api.UserItemInfo, error) {
	return call(ctx, "UpdateUser", func(ctx context.Context) (*api.UserItemInfo, error) {
		return i.rw.UpdateUser(ctx, id, username, expireAt, squad)
	})
}

// Users: один спан и одна метрика на весь проход итератора.
func (i instrumented) Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error] {
	if isProbe(ctx) {
		return i.rw.Users(ctx)
	}
	return func(yield func(api.UserItemInfo, error) bool) {
		ctx, span := startSpan(ctx, "remnawave Users")
		start := time.Now()
		var failed error
		defer func() { finish(span, "remnawave", "Users", start, failed) }()
		for u, err := range i.rw.Users(ctx) {
			if err != nil {
				failed = err
			}
			if !yield(u, err) {
				return
			}
		}
	}
}
