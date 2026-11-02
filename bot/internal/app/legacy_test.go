package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func TestWarnLegacyPayments(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.HSet("fsm:111", "state", "in_review")
	mr.HSet("fsm:222", "state", "none")
	mr.HSet("other:333", "state", "in_review")

	var buf bytes.Buffer
	ctx := stlog.Wrap(t.Context(), slog.New(slog.NewTextHandler(&buf, nil)))
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	if err := warnLegacyPayments(ctx, rdb); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, "level=WARN"); n != 1 {
		t.Fatalf("warns = %d, log:\n%s", n, out)
	}
	if !strings.Contains(out, "telegram_id=111") {
		t.Fatalf("no telegram_id=111:\n%s", out)
	}
}
