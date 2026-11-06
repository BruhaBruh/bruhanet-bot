package store

import (
	"testing"
)

func TestTextOverrides(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()

	m, err := s.TextOverrides(ctx)
	if err != nil || len(m) != 0 {
		t.Fatalf("empty: %v %v", m, err)
	}
	if err = s.SetText(ctx, "a", "one"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetText(ctx, "a", "two"); err != nil { // upsert
		t.Fatal(err)
	}
	if err = s.SetText(ctx, "b", "bee"); err != nil {
		t.Fatal(err)
	}
	m, err = s.TextOverrides(ctx)
	if err != nil || len(m) != 2 || m["a"] != "two" || m["b"] != "bee" {
		t.Fatalf("got %v %v", m, err)
	}
	if err = s.ResetText(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetText(ctx, "missing"); err != nil { // идемпотентно
		t.Fatal(err)
	}
	m, _ = s.TextOverrides(ctx)
	if len(m) != 1 || m["b"] != "bee" {
		t.Fatalf("after reset: %v", m)
	}
}

func TestTextStyles(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	success := "success"
	if err := s.SetTextStyle(ctx, "button.pay", &success); err != nil {
		t.Fatal(err)
	}
	// стиль без правки подписи — не переопределение текста
	if m, err := s.TextOverrides(ctx); err != nil || len(m) != 0 {
		t.Fatalf("overrides = %v %v", m, err)
	}
	if m, err := s.TextStyles(ctx); err != nil || m["button.pay"] != "success" {
		t.Fatalf("styles = %v %v", m, err)
	}
	// подпись и стиль независимы
	if err := s.SetText(ctx, "button.pay", "Платить"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearTextBody(ctx, "button.pay"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.TextOverrides(ctx); len(m) != 0 {
		t.Fatalf("overrides = %v", m)
	}
	if m, _ := s.TextStyles(ctx); m["button.pay"] != "success" {
		t.Fatalf("style lost: %v", m)
	}
	// «Обычная» — пустая строка, не то же, что нет стиля
	plain := ""
	if err := s.SetTextStyle(ctx, "button.buy", &plain); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.TextStyles(ctx); m["button.buy"] != "" || len(m) != 2 {
		t.Fatalf("styles = %v", m)
	}
	if err := s.SetTextStyle(ctx, "button.buy", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ResetText(ctx, "button.pay"); err != nil {
		t.Fatal(err)
	}
	if m, _ := s.TextStyles(ctx); len(m) != 0 {
		t.Fatalf("styles = %v", m)
	}
}

func TestSettingsRaw(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()

	if err := s.SetSetting(ctx, "phone", []byte(`"+7 900"`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "phone", []byte(`"+7 911"`)); err != nil { // upsert
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "prices", []byte(`{"1":100}`)); err != nil {
		t.Fatal(err)
	}
	m, err := s.SettingsRaw(ctx)
	if err != nil || len(m) != 2 || string(m["phone"]) != `"+7 911"` {
		t.Fatalf("got %v %v", m, err)
	}
	if err = s.SetSetting(ctx, "bad", []byte(`{`)); err == nil {
		t.Fatal("invalid json must fail")
	}
}

func TestSetSettingsAtomic(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if err := s.SetSettings(ctx, map[string][]byte{"phone": []byte(`"a"`), "bank": []byte(`"b"`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(ctx, map[string][]byte{"phone": []byte(`"z"`), "bad": []byte(`{`)}); err == nil {
		t.Fatal("invalid json must fail")
	}
	m, err := s.SettingsRaw(ctx)
	if err != nil || string(m["phone"]) != `"a"` || len(m) != 2 {
		t.Fatalf("rolled back: %v %v", m, err)
	}
}
