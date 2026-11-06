package routes

import (
	"context"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const writeThroughTimeout = 2 * time.Second

// writeThrough после успешного вызова панели сразу пишет срок и статус в users: единая точка для всех
// хендлеров. Панель — источник правды, сбой записи не роняет операцию, её дочинит синхронизация.
type writeThrough struct {
	Remnawave
	pay Payments
}

func (w writeThrough) CreateUser(ctx context.Context, id int64, username string, squad uuid.UUID) (*api.UserItemInfo, error) {
	u, err := w.Remnawave.CreateUser(ctx, id, username, squad)
	if err == nil {
		w.save(ctx, id, u)
	}
	return u, err
}

func (w writeThrough) UpdateUser(ctx context.Context, id int64, username string, expireAt time.Time, squad uuid.UUID) (*api.UserItemInfo, error) {
	u, err := w.Remnawave.UpdateUser(ctx, id, username, expireAt, squad)
	if err == nil {
		w.save(ctx, id, u)
	}
	return u, err
}

func (w writeThrough) save(ctx context.Context, id int64, u *api.UserItemInfo) {
	// Таймаут: зов идёт и из транзакции, которая держит соединение пула.
	ctx, cancel := context.WithTimeout(ctx, writeThroughTimeout)
	defer cancel()
	if err := w.pay.SetUserPanel(ctx, id, u.ExpireAt, string(u.Status), time.Now()); err != nil {
		stlog.Warn(ctx, "failed to save panel state", "error", err)
	}
}
