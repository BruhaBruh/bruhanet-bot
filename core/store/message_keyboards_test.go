package store

import (
	"testing"

	"github.com/bruhabruh/bruhanet/core/texts"
)

func TestMessageKeyboards(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	def, _, _ := texts.MessageDefault("payment.confirm")
	def.Rows[0][0].Label = "Верно"
	if err := s.SaveMessageKeyboard(ctx, "payment.confirm", def); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMessageKeyboard(ctx, "payment.confirm", def); err != nil { // upsert
		t.Fatal(err)
	}
	got, err := s.MessageKeyboards(ctx)
	if err != nil || len(got) != 1 || !got["payment.confirm"].Equal(def) {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}
	if err = s.DeleteMessageKeyboard(ctx, "payment.confirm"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteMessageKeyboard(ctx, "payment.confirm"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.MessageKeyboards(ctx); len(got) != 0 {
		t.Fatalf("after delete: %v", got)
	}
}

func TestMigrateLegacyKeyboards(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if n, err := s.MigrateLegacyKeyboards(ctx); err != nil || n != 0 {
		t.Fatalf("empty: %d %v", n, err)
	}
	// общая клавиатура «reminder» стоит под двумя сообщениями, у одного из них уже есть своя
	if err := s.SaveKeyboardLayout(ctx, "reminder", texts.Layout{Rows: [][]string{{"button.subscription"}}, Hidden: []string{"button.renew"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveKeyboardLayout(ctx, "referral", texts.Layout{Rows: [][]string{{"button.back"}, {"button.copy_link"}}}); err != nil {
		t.Fatal(err)
	}
	own, _, _ := texts.MessageDefault("reminder.day")
	own.Rows = own.Rows[:1]
	own.Rows[0] = own.Rows[0][:1]
	if err := s.SaveMessageKeyboard(ctx, "reminder.day", own); err != nil {
		t.Fatal(err)
	}
	n, err := s.MigrateLegacyKeyboards(ctx)
	if err != nil || n != 2 { // reminder.week и referral.menu
		t.Fatalf("migrate: %d %v", n, err)
	}
	got, _ := s.MessageKeyboards(ctx)
	if !got["reminder.day"].Equal(own) {
		t.Errorf("own keyboard overwritten: %+v", got["reminder.day"])
	}
	if w := got["reminder.week"]; len(w.Rows) != 1 || w.Rows[0][0].ID != "subscription" {
		t.Errorf("week: %+v", w)
	}
	if r := got["referral.menu"]; len(r.Rows) != 2 || r.Rows[0][0].ID != "back" {
		t.Errorf("referral: %+v", r)
	}
	for key, l := range got {
		if err := texts.ValidateMessageLayout(key, l, nil, false); err != nil {
			t.Errorf("%s migrated invalid: %v", key, err)
		}
	}
	if left, _ := s.KeyboardLayouts(ctx); len(left) != 0 {
		t.Errorf("legacy rows left: %v", left)
	}
	// повтор после сброса клавиатуры сообщения не воскрешает её
	if err = s.DeleteMessageKeyboard(ctx, "referral.menu"); err != nil {
		t.Fatal(err)
	}
	if n, err = s.MigrateLegacyKeyboards(ctx); err != nil || n != 0 {
		t.Fatalf("rerun: %d %v", n, err)
	}
	if got, _ = s.MessageKeyboards(ctx); len(got) != 2 {
		t.Errorf("rerun changed rows: %v", got)
	}
}
