package ratelimit_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/bot/internal/ratelimit"
)

func TestFailsCountPerKeyAndExpire(t *testing.T) {
	mr := miniredis.RunT(t)
	l := ratelimit.New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := t.Context()
	if n, err := l.Fails(ctx, "k"); err != nil || n != 0 {
		t.Fatalf("empty = %d %v", n, err)
	}
	for range 3 {
		if err := l.Fail(ctx, "k", time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := l.Fails(ctx, "k"); n != 3 {
		t.Fatalf("fails = %d", n)
	}
	if n, _ := l.Fails(ctx, "other"); n != 0 {
		t.Fatalf("other key = %d", n)
	}
	mr.FastForward(2 * time.Hour) // окно закончилось: счёт с нуля
	if n, _ := l.Fails(ctx, "k"); n != 0 {
		t.Fatalf("after the window = %d", n)
	}
}
