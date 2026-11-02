package routes

import (
	"slices"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

func TestFormatStats(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	at := func(t time.Time) *time.Time { return &t }
	for _, tc := range []struct {
		name string
		in   model.UserStats
		want []string
	}{
		{"new", model.UserStats{}, []string{"🆕 первая оплата"}},
		{
			"regular",
			model.UserStats{
				Confirmed: 4, ConfirmedSum: 2350, UnpaidAdvances: 1, HasHistory: true,
				// 21:30 UTC 11.09 = 00:30 МСК 12.09
				LastConfirmedAt:  at(time.Date(2026, time.September, 11, 21, 30, 0, 0, time.UTC)),
				FirstConfirmedAt: at(time.Date(2025, time.March, 5, 10, 0, 0, 0, msk)),
			},
			[]string{
				"📊 Оплат: 4 на 2 350 ₽ · последняя 12\\.09\\.2026",
				"❌ Авансов без оплаты: 1 из 3",
				"🕐 Клиент с 03\\.2025",
			},
		},
		{
			"limit without confirmed",
			model.UserStats{UnpaidAdvances: 3, HasHistory: true},
			[]string{"📊 Оплат: 0 на 0 ₽", "⚠️ Авансов без оплаты: 3 — без аванса"},
		},
		{
			"big sum",
			model.UserStats{
				Confirmed: 120, ConfirmedSum: 1234567, HasHistory: true,
				LastConfirmedAt:  at(time.Date(2026, time.January, 2, 12, 0, 0, 0, msk)),
				FirstConfirmedAt: at(time.Date(2024, time.December, 31, 23, 0, 0, 0, msk)),
			},
			[]string{
				"📊 Оплат: 120 на 1 234 567 ₽ · последняя 02\\.01\\.2026",
				"❌ Авансов без оплаты: 0 из 3",
				"🕐 Клиент с 12\\.2024",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatStats(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("formatStats = %q, want %q", got, tc.want)
			}
		})
	}
}
