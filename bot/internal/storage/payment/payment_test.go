package payment_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/storage/payment"
)

func client() *redis.Client {
	db, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	return redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_ADDR"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       db,
	})
}

func TestCreate(t *testing.T) {
	rdb := client()
	defer rdb.Close()

	ps := payment.New(rdb)
	ctx := context.Background()

	if err := ps.Create(ctx, &model.Payment{}); err != nil {
		t.Fatalf("ожидалось что ps.Create не вернет ошибку")
	}

	ps.Delete(ctx, 0)
}

func TestGet(t *testing.T) {
	rdb := client()
	defer rdb.Close()

	ps := payment.New(rdb)
	ctx := context.Background()

	payment := &model.Payment{
		TelegramID: rand.Int64(),
	}
	if err := ps.Create(ctx, payment); err != nil {
		t.Fatalf("ожидалось что ps.Create не вернет ошибку")
	}
	p, err := ps.Get(ctx, payment.TelegramID)
	if err != nil {
		t.Fatalf("ожидалось что ps.Get не вернет ошибку")
	}
	if p.TelegramID != payment.TelegramID {
		t.Fatalf("вернуло неверный payment")
	}

	ps.Delete(ctx, payment.TelegramID)
}

func TestGetNotFound(t *testing.T) {
	rdb := client()
	defer rdb.Close()

	ps := payment.New(rdb)
	ctx := context.Background()

	_, err := ps.Get(ctx, rand.Int64())
	if err != nil && !errors.Is(err, payment.ErrNotFound) {
		t.Fatalf("ожидалось что ps.Get вернет not found ошибку")
	}
	if err == nil {
		t.Fatalf("ожидалось что ps.Get вернет ошибку")
	}
}

func TestDelete(t *testing.T) {
	rdb := client()
	defer rdb.Close()

	ps := payment.New(rdb)
	ctx := context.Background()

	payment := &model.Payment{
		TelegramID: rand.Int64(),
	}
	if err := ps.Create(ctx, payment); err != nil {
		t.Fatalf("ожидалось что ps.Create не вернет ошибку")
	}
	err := ps.Delete(ctx, payment.TelegramID)
	if err != nil {
		t.Fatalf("ожидалось что ps.Get не вернет ошибку")
	}
}
