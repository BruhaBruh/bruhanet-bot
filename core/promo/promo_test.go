package promo

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

func ptr[T any](v T) *T { return &v }

func TestNormalize(t *testing.T) {
	if got := Normalize(" summer25 "); got != "SUMMER25" {
		t.Fatalf("Normalize = %q", got)
	}
}

func TestGenerate(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		c := Generate()
		if len(c) != 8 {
			t.Fatalf("len(%q) = %d", c, len(c))
		}
		for _, r := range c {
			if !strings.ContainsRune(alphabet, r) {
				t.Fatalf("%q: rune %q not in alphabet", c, r)
			}
		}
		if err := ValidateNew(model.Promo{Code: c, Kind: KindDays, Value: 1}); err != nil {
			t.Fatalf("generated %q invalid: %v", c, err)
		}
		seen[c] = true
	}
	if len(seen) < 95 {
		t.Fatalf("too many duplicates: %d unique", len(seen))
	}
	if alphabet != "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" {
		t.Fatalf("alphabet = %q", alphabet)
	}
}

func TestValidateNew(t *testing.T) {
	ok := model.Promo{Code: "SUMMER_25-x", Kind: KindPercent, Value: 25}
	tests := []struct {
		name string
		mod  func(*model.Promo)
		ok   bool
	}{
		{"valid lower normalized", func(*model.Promo) {}, true},
		{"code short", func(p *model.Promo) { p.Code = "AB" }, false},
		{"code min", func(p *model.Promo) { p.Code = "ABC" }, true},
		{"code max", func(p *model.Promo) { p.Code = strings.Repeat("A", 32) }, true},
		{"code long", func(p *model.Promo) { p.Code = strings.Repeat("A", 33) }, false},
		{"code bad char", func(p *model.Promo) { p.Code = "SUM MER" }, false},
		{"code cyrillic", func(p *model.Promo) { p.Code = "ЛЕТО25" }, false},
		{"bad kind", func(p *model.Promo) { p.Kind = "gift" }, false},
		{"percent 0", func(p *model.Promo) { p.Value = 0 }, false},
		{"percent 100", func(p *model.Promo) { p.Value = 100 }, true},
		{"percent 101", func(p *model.Promo) { p.Value = 101 }, false},
		{"rub 1", func(p *model.Promo) { p.Kind, p.Value = KindRub, 1 }, true},
		{"rub max", func(p *model.Promo) { p.Kind, p.Value = KindRub, 1_000_000 }, true},
		{"rub over", func(p *model.Promo) { p.Kind, p.Value = KindRub, 1_000_001 }, false},
		{"days 0", func(p *model.Promo) { p.Kind, p.Value = KindDays, 0 }, false},
		{"days 3650", func(p *model.Promo) { p.Kind, p.Value = KindDays, 3650 }, true},
		{"days 3651", func(p *model.Promo) { p.Kind, p.Value = KindDays, 3651 }, false},
		{"max activations 0", func(p *model.Promo) { p.MaxActivations = ptr(0) }, false},
		{"max activations 1", func(p *model.Promo) { p.MaxActivations = ptr(1) }, true},
		{"telegram id 0", func(p *model.Promo) { p.TelegramID = ptr[int64](0) }, false},
		{"telegram id", func(p *model.Promo) { p.TelegramID = ptr[int64](42) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ok
			p.Code = strings.ToLower(p.Code)
			tt.mod(&p)
			err := ValidateNew(p)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	const me = int64(7)
	base := model.Promo{Code: "X", Kind: KindPercent, Value: 10}
	type in struct {
		p         model.Promo
		used      int
		confirmed bool
		activated bool
		place     Place
	}
	tests := []struct {
		name string
		in   in
		want error
	}{
		{"ok discount at payment", in{p: base, place: PlacePayment}, nil},
		{"ok days in menu", in{p: model.Promo{Kind: KindDays, Value: 3}, place: PlaceMenu}, nil},
		{"disabled", in{p: model.Promo{Kind: KindPercent, Disabled: true}, place: PlacePayment}, ErrDisabled},
		{"disabled beats expired", in{p: model.Promo{Kind: KindPercent, Disabled: true, ExpiresAt: ptr(now.Add(-time.Hour))}, place: PlacePayment}, ErrDisabled},
		{"expired", in{p: model.Promo{Kind: KindPercent, ExpiresAt: ptr(now.Add(-time.Hour))}, place: PlacePayment}, ErrExpired},
		{"expires exactly now", in{p: model.Promo{Kind: KindPercent, ExpiresAt: ptr(now)}, place: PlacePayment}, ErrExpired},
		{"not yet expired", in{p: model.Promo{Kind: KindPercent, ExpiresAt: ptr(now.Add(time.Second))}, place: PlacePayment}, nil},
		{"expired beats exhausted", in{p: model.Promo{Kind: KindPercent, ExpiresAt: ptr(now), MaxActivations: ptr(1)}, used: 1, place: PlacePayment}, ErrExpired},
		{"exhausted", in{p: model.Promo{Kind: KindPercent, MaxActivations: ptr(2)}, used: 2, place: PlacePayment}, ErrExhausted},
		{"limit left", in{p: model.Promo{Kind: KindPercent, MaxActivations: ptr(2)}, used: 1, place: PlacePayment}, nil},
		{"exhausted beats not yours", in{p: model.Promo{Kind: KindPercent, MaxActivations: ptr(1), TelegramID: ptr[int64](8)}, used: 1, place: PlacePayment}, ErrExhausted},
		{"not yours", in{p: model.Promo{Kind: KindPercent, TelegramID: ptr[int64](8)}, place: PlacePayment}, ErrNotYours},
		{"personal mine", in{p: model.Promo{Kind: KindPercent, TelegramID: ptr(me)}, place: PlacePayment}, nil},
		{"not yours beats only new", in{p: model.Promo{Kind: KindPercent, TelegramID: ptr[int64](8), OnlyNew: true}, confirmed: true, place: PlacePayment}, ErrNotYours},
		{"only new", in{p: model.Promo{Kind: KindPercent, OnlyNew: true}, confirmed: true, place: PlacePayment}, ErrOnlyNew},
		{"only new and new", in{p: model.Promo{Kind: KindPercent, OnlyNew: true}, place: PlacePayment}, nil},
		{"only new beats already used", in{p: model.Promo{Kind: KindPercent, OnlyNew: true}, confirmed: true, activated: true, place: PlacePayment}, ErrOnlyNew},
		{"already used", in{p: base, activated: true, place: PlacePayment}, ErrAlreadyUsed},
		{"already used beats wrong place", in{p: base, activated: true, place: PlaceMenu}, ErrAlreadyUsed},
		{"discount in menu", in{p: base, place: PlaceMenu}, ErrWrongPlace},
		{"rub in menu", in{p: model.Promo{Kind: KindRub, Value: 10}, place: PlaceMenu}, ErrWrongPlace},
		{"days at payment", in{p: model.Promo{Kind: KindDays, Value: 3}, place: PlacePayment}, ErrWrongPlace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(tt.in.p, tt.in.used, me, tt.in.confirmed, tt.in.activated, tt.in.place, now)
			if tt.want == nil && err != nil || tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("Check = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		base int
		p    model.Promo
		want int
	}{
		{150, model.Promo{Kind: KindPercent, Value: 25}, 112},
		{150, model.Promo{Kind: KindPercent, Value: 100}, 0},
		{150, model.Promo{Kind: KindPercent, Value: 1}, 148},
		{150, model.Promo{Kind: KindRub, Value: 200}, 0},
		{150, model.Promo{Kind: KindRub, Value: 50}, 100},
		{150, model.Promo{Kind: KindDays, Value: 5}, 150},
	}
	for _, tt := range tests {
		if got := Apply(tt.base, tt.p); got != tt.want {
			t.Errorf("Apply(%d, %+v) = %d, want %d", tt.base, tt.p, got, tt.want)
		}
	}
}
