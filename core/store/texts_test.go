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
