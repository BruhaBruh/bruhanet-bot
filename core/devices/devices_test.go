package devices

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func TestRemainingDays(t *testing.T) {
	for name, tc := range map[string]struct {
		expire time.Time
		want   int
	}{
		"expired":     {now.Add(-time.Hour), 0},
		"exactly now": {now, 0},
		"one second":  {now.Add(time.Second), 1},
		"one day":     {now.Add(24 * time.Hour), 1},
		"day + 1s":    {now.Add(24*time.Hour + time.Second), 2},
		"30 days":     {now.Add(30 * 24 * time.Hour), 30},
	} {
		if got := RemainingDays(tc.expire, now); got != tc.want {
			t.Errorf("%s: %d, want %d", name, got, tc.want)
		}
	}
}

func TestProratedPrice(t *testing.T) {
	for name, tc := range map[string]struct {
		price, count int
		left         time.Duration
		want         int
	}{
		"full month":      {90, 1, 30 * 24 * time.Hour, 90},
		"half month":      {90, 1, 15 * 24 * time.Hour, 45},
		"round up":        {100, 1, 10 * 24 * time.Hour, 34}, // 33.33 -> 34
		"last day":        {90, 1, time.Hour, 3},
		"minimum 1 rub":   {1, 1, time.Hour, 1},
		"several":         {50, 3, 20 * 24 * time.Hour, 100},
		"two months left": {60, 2, 60 * 24 * time.Hour, 240},
	} {
		got, err := ProratedPrice(tc.price, tc.count, now.Add(tc.left), now)
		if err != nil || got != tc.want {
			t.Errorf("%s: %d %v, want %d", name, got, err, tc.want)
		}
	}
	for name, exp := range map[string]time.Time{"expired": now.Add(-time.Minute), "zero": {}, "now": now} {
		if _, err := ProratedPrice(50, 1, exp, now); !errors.Is(err, ErrNoTerm) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTariffAddon(t *testing.T) {
	if got := TariffAddon(50, 2, 3); got != 300 {
		t.Fatal(got)
	}
	if TariffAddon(50, 0, 3) != 0 {
		t.Fatal("no extra")
	}
}

func TestRoom(t *testing.T) {
	for _, tc := range []struct{ base, extra, want int }{{5, 0, 95}, {5, 95, 0}, {5, 94, 1}, {100, 0, 0}, {5, 200, 0}} {
		if got := Room(tc.base, tc.extra); got != tc.want {
			t.Errorf("Room(%d, %d) = %d, want %d", tc.base, tc.extra, got, tc.want)
		}
	}
}
