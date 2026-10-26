package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
)

type paymentStorage struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *paymentStorage {
	return &paymentStorage{
		rdb: rdb,
	}
}

func (s *paymentStorage) Create(ctx context.Context, payment *model.Payment) error {
	data, err := json.Marshal(payment)
	if err != nil {
		return errors.Join(ErrFailedToMarshal, err)
	}
	return s.rdb.Set(ctx, s.paymentKey(payment.TelegramID), data, 0).Err()
}

func (ps *paymentStorage) Get(ctx context.Context, telegramID int64) (*model.Payment, error) {
	val, err := ps.rdb.Get(ctx, ps.paymentKey(telegramID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var payment model.Payment
	err = json.Unmarshal([]byte(val), &payment)
	if err != nil {
		return nil, errors.Join(ErrFailedToUnmarshal, err)
	}
	return &payment, nil
}

func (s *paymentStorage) Delete(ctx context.Context, telegramID int64) error {
	return s.rdb.Del(ctx, s.paymentKey(telegramID)).Err()
}

func (ps *paymentStorage) paymentKey(telegramID int64) string {
	return fmt.Sprintf("payment:%d", telegramID)
}
