package texts_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/core/usererr"
)

// Реестр раскладок согласован с реестром текстов и сам проходит свою проверку.
func TestKeyboardRegistry(t *testing.T) {
	ids := map[string]bool{}
	for _, k := range texts.Keyboards() {
		if ids[k.ID] {
			t.Errorf("duplicate keyboard %s", k.ID)
		}
		ids[k.ID] = true
		if len(k.Messages) == 0 {
			t.Errorf("%s: no messages", k.ID)
		}
		if err := k.ValidateLayout(texts.Layout{Rows: k.Default}); err != nil {
			t.Errorf("%s: default layout: %v", k.ID, err)
		}
		inDefault := slices.Concat(k.Default...)
		for _, s := range k.Slots {
			d, ok := texts.Lookup(s.ID)
			if !ok || d.Kind != texts.KindButton {
				t.Errorf("%s: slot %s is not a registry button", k.ID, s.ID)
			}
			if !slices.Contains(inDefault, s.ID) {
				t.Errorf("%s: slot %s is not in the default layout", k.ID, s.ID)
			}
		}
		for _, id := range inDefault {
			if _, isSlot := slotOf(k, id); !isSlot && !blockOf(k, id) {
				t.Errorf("%s: default has undeclared %s", k.ID, id)
			}
		}
		for _, m := range k.Messages {
			d, ok := texts.Lookup(m)
			if !ok || d.Kind != texts.KindMessage {
				t.Errorf("%s: %s is not a message", k.ID, m)
				continue
			}
			for _, s := range k.Slots {
				if !slices.Contains(d.Buttons, s.ID) {
					t.Errorf("%s: message %s does not declare button %s", k.ID, m, s.ID)
				}
			}
		}
	}
}

func slotOf(k texts.Keyboard, id string) (texts.Slot, bool) {
	for _, s := range k.Slots {
		if s.ID == id {
			return s, true
		}
	}
	return texts.Slot{}, false
}

func blockOf(k texts.Keyboard, id string) bool {
	return slices.ContainsFunc(k.Blocks, func(b texts.Block) bool { return b.ID == id })
}

func kbOf(t *testing.T, id string) texts.Keyboard {
	t.Helper()
	k, ok := texts.LookupKeyboard(id)
	if !ok {
		t.Fatalf("no keyboard %s", id)
	}
	return k
}

func TestValidateLayout(t *testing.T) {
	menu, tariffs := kbOf(t, "menu"), kbOf(t, "tariffs")
	all := func(rows ...[]string) texts.Layout { return texts.Layout{Rows: rows} }
	tests := []struct {
		name string
		k    texts.Keyboard
		l    texts.Layout
		ok   bool
	}{
		{"default", menu, all(menu.Default...), true},
		{"reorder and two per row", menu, all(
			[]string{"button.pay", "button.info"}, []string{"button.support", "button.subscription"}, []string{"button.trial"},
			[]string{"button.promo", "button.devices"}, []string{"button.gift"}, []string{"button.referral"}), true},
		{"three in a row", menu, all([]string{"button.info", "button.pay", "button.promo"}, []string{"button.subscription", "button.support"}, []string{"button.trial", "button.devices", "button.gift", "button.referral"}), false},
		{"duplicate", menu, all(menu.Default[0], []string{"button.info"}), false},
		{"unknown", menu, all([]string{"button.nope"}), false},
		{"other keyboard's button", menu, all([]string{"button.back"}), false},
		{"empty row", menu, all([]string{}), false},
		{"nine rows", menu, all(
			[]string{"button.info"}, []string{"button.pay"}, []string{"button.subscription"}, []string{"button.support"},
			[]string{"button.trial"}, []string{"button.promo"}, []string{"button.devices"}, []string{"button.gift"}, []string{"button.referral"}), false},
		{"hide removable", menu, texts.Layout{Rows: [][]string{{"button.info"}}, Hidden: []string{"button.pay", "button.promo"}}, true},
		{"hidden listed in rows too", menu, texts.Layout{Rows: [][]string{{"button.info", "button.pay"}}, Hidden: []string{"button.pay"}}, true},
		{"row empty after hiding", menu, texts.Layout{Rows: [][]string{{"button.pay"}, {"button.info"}}, Hidden: []string{"button.pay"}}, false},
		{"hide twice", menu, texts.Layout{Hidden: []string{"button.pay", "button.pay"}}, false},
		{"hide unknown", menu, texts.Layout{Hidden: []string{"button.back"}}, false},
		{"hide non-removable", kbOf(t, "confirm"), texts.Layout{Rows: [][]string{{"button.no"}}, Hidden: []string{"button.yes"}}, false},
		{"non-removable missing", kbOf(t, "confirm"), all([]string{"button.yes"}), false},
		{"block moved", tariffs, all([]string{"button.back"}, []string{"@tariffs"}), true},
		{"block with a neighbour", tariffs, all([]string{"@tariffs", "button.back"}), false},
		{"block missing", tariffs, all([]string{"button.back"}), false},
		{"block cannot be hidden", tariffs, texts.Layout{Rows: [][]string{{"button.back"}, {"@tariffs"}}, Hidden: []string{"@tariffs"}}, false},
		{"block of another keyboard", kbOf(t, "confirm"), all([]string{"@tariffs"}), false},
	}
	for _, tc := range tests {
		err := tc.k.ValidateLayout(tc.l)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
		if err != nil {
			if !errors.Is(err, texts.ErrBadLayout) || usererr.Message(err) == usererr.Fallback {
				t.Errorf("%s: no sentinel or Russian message: %v", tc.name, err)
			}
		}
	}
}

func btns(ids ...string) map[string][][]string {
	m := map[string][][]string{}
	for _, id := range ids {
		m[id] = [][]string{{id}}
	}
	return m
}

func TestArrange(t *testing.T) {
	menu := kbOf(t, "menu")
	allMenu := []string{"button.info", "button.pay", "button.subscription", "button.support", "button.trial", "button.promo", "button.devices", "button.gift", "button.referral"}
	eq := func(name string, got, want [][]string) {
		t.Helper()
		if !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	eq("default", texts.Arrange(menu, nil, btns(allMenu...)), menu.Default)
	// условие в коде не выполнено: кнопки нет, пустой ряд исчезает, соседка остаётся
	eq("absent", texts.Arrange(menu, nil, btns("button.info", "button.pay", "button.promo")),
		[][]string{{"button.info", "button.pay"}, {"button.promo"}})
	// переставили и поставили в два столбца
	l := &texts.Layout{Rows: [][]string{{"button.gift", "button.referral"}, {"button.pay", "button.info"}}}
	eq("reordered", texts.Arrange(menu, l, btns("button.info", "button.pay", "button.gift", "button.referral")),
		[][]string{{"button.gift", "button.referral"}, {"button.pay", "button.info"}})
	// скрытая не показывается, даже если код её построил
	l = &texts.Layout{Rows: [][]string{{"button.info"}}, Hidden: []string{"button.pay"}}
	eq("hidden", texts.Arrange(menu, l, btns("button.info", "button.pay")), [][]string{{"button.info"}})
	// новая кнопка кода, которой нет в сохранённой раскладке, встаёт в конец
	l = &texts.Layout{Rows: [][]string{{"button.info"}}}
	eq("new code button", texts.Arrange(menu, l, btns("button.info", "button.pay", "button.gift")),
		[][]string{{"button.info"}, {"button.pay"}, {"button.gift"}})
	// блок раскрывается своими рядами на месте, раскладка может поставить его после «Назад»
	tariffs := kbOf(t, "tariffs")
	built := map[string][][]string{"@tariffs": {{"a", "b"}, {"c"}}, "button.back": {{"button.back"}}}
	eq("block default", texts.Arrange(tariffs, nil, built), [][]string{{"a", "b"}, {"c"}, {"button.back"}})
	l = &texts.Layout{Rows: [][]string{{"button.back"}, {"@tariffs"}}}
	eq("block moved", texts.Arrange(tariffs, l, built), [][]string{{"button.back"}, {"a", "b"}, {"c"}})
}

func TestEffective(t *testing.T) {
	menu := kbOf(t, "menu")
	got := menu.Effective(nil)
	if !slices.EqualFunc(got.Rows, menu.Default, slices.Equal) || len(got.Hidden) != 0 {
		t.Fatalf("default: %+v", got)
	}
	got = menu.Effective(&texts.Layout{Rows: [][]string{{"button.pay", "button.info"}}, Hidden: []string{"button.referral", "button.gift"}})
	if got.Rows[0][0] != "button.pay" || slices.Contains(slices.Concat(got.Rows...), "button.gift") || len(got.Hidden) != 2 {
		t.Fatalf("custom: %+v", got)
	}
	// остальные кнопки (не хранившиеся) не пропали
	if !slices.Contains(slices.Concat(got.Rows...), "button.support") {
		t.Fatalf("missing slot dropped: %+v", got)
	}
}
