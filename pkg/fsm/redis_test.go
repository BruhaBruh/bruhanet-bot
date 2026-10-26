package fsm_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestRedis(t *testing.T) {
	testStorage(t, func(t *testing.T) fsm.Storage {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = rdb.Close() })
		return fsm.NewRedis(rdb)
	})
}

func TestRedisKeyLayout(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := fsm.NewRedis(rdb).Set(t.Context(), 42, fsm.Record{State: "a", Data: []byte("1")}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := mr.HGet("fsm:42", "state"); got != "a" {
		t.Fatalf("fsm:42 state = %q", got)
	}
	if got := mr.HGet("fsm:42", "data"); got != "1" {
		t.Fatalf("fsm:42 data = %q", got)
	}
	if mr.TTL("fsm:42") != 0 {
		t.Fatal("у ключа FSM не должно быть TTL")
	}
}
