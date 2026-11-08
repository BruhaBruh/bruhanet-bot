package texts_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/core/usererr"
)

// Каждая клавиатура по умолчанию проходит собственную проверку и совпадает с кодовой раскладкой.
func TestMessageDefaults(t *testing.T) {
	msgs := texts.MessagesWithKeyboard()
	if len(msgs) == 0 {
		t.Fatal("no messages with keyboards")
	}
	for _, key := range msgs {
		def, locked, ok := texts.MessageDefault(key)
		if !ok {
			t.Fatalf("%s: no default", key)
		}
		if err := texts.ValidateMessageLayout(key, def, nil, true); err != nil {
			t.Errorf("%s: default is invalid: %v", key, err)
		}
		// без кнопок по умолчанию (0 клавиатур): реквизиты оплаты — админ может добавить свои
		if n := len(texts.KeyboardsFor(key)); n > 2 || (n == 2 && texts.KeyboardsFor(key)[0].ID != "menu") {
			t.Errorf("%s: %d keyboards", key, n)
		}
		for _, id := range locked {
			if !slices.ContainsFunc(cellsOf(def), func(c texts.Cell) bool { return c.ID == id }) {
				t.Errorf("%s: locked %s is not in default", key, id)
			}
		}
	}
}

func cellsOf(l texts.MsgLayout) []texts.Cell { return slices.Concat(l.Rows...) }

// Отфильтрованное условием «Подписка заведена» меню по умолчанию совпадает с каждой из двух кодовых клавиатур.
func TestMenuDefaultMatchesBothKeyboards(t *testing.T) {
	def, _, _ := texts.MessageDefault("menu.greeting")
	for _, tc := range []struct {
		kb      string
		inPanel bool
	}{{"menu", true}, {"menu_new", false}} {
		k, _ := texts.LookupKeyboard(tc.kb)
		var got [][]string
		for _, row := range def.Rows {
			var r []string
			for _, c := range row {
				show := true
				for _, cr := range c.Conditions {
					if tc.inPanel == cr.Not {
						show = false
					}
				}
				if show {
					r = append(r, c.Key)
				}
			}
			if len(r) > 0 {
				got = append(got, r)
			}
		}
		// кнопки, которых кодовая клавиатура строит по условиям (пробный, подарок, …), в сравнении все
		want := k.Default
		if !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("%s: got %v want %v", tc.kb, got, want)
		}
	}
}

func valid(t *testing.T, key string, mutate func(l *texts.MsgLayout)) error {
	t.Helper()
	def, _, _ := texts.MessageDefault(key)
	l := def.Clone()
	mutate(&l)
	return texts.ValidateMessageLayout(key, l, nil, true)
}

func TestValidateMessageLayout(t *testing.T) {
	custom := texts.Cell{ID: "site", Label: "Сайт", Action: "url:https://example.com", Conditions: []texts.CondRef{}}
	cases := []struct {
		name   string
		key    string
		mut    func(l *texts.MsgLayout)
		wantOK bool
	}{
		{"default", "menu.greeting", func(l *texts.MsgLayout) {}, true},
		{"add custom url button", "menu.greeting", func(l *texts.MsgLayout) { l.Rows = append(l.Rows, []texts.Cell{custom}) }, true},
		{"http url", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Action = "url:http://example.com"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"javascript url", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Action = "url:javascript:alert(1)"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"three in a row", "menu.greeting", func(l *texts.MsgLayout) { l.Rows[2] = append(l.Rows[2], custom) }, false},
		{"nine rows", "menu.greeting", func(l *texts.MsgLayout) {
			for _, id := range []string{"n1", "n2", "n3"} { // 6 рядов по умолчанию + 3 = 9
				c := custom
				c.ID = id
				l.Rows = append(l.Rows, []texts.Cell{c})
			}
		}, false},
		{"empty row", "menu.greeting", func(l *texts.MsgLayout) { l.Rows = append(l.Rows, nil) }, false},
		{"duplicate id", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.ID = "gift"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"bad id", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.ID = "Bad ID"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"unknown action", "menu.greeting", func(l *texts.MsgLayout) { l.Rows[2][0].Action = "route:admin" }, false},
		{"unknown condition", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[5][0].Conditions = []texts.CondRef{{ID: "nope"}}
		}, false},
		{"condition twice / contradiction", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[5][0].Conditions = []texts.CondRef{{ID: "gifts_on"}, {ID: "gifts_on", Not: true}}
		}, false},
		{"contradicts action guard", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[0][1].Conditions = []texts.CondRef{{ID: "trial_available", Not: true}}
		}, false},
		{"not condition ok", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[0][1].Conditions = []texts.CondRef{{ID: "referral_on", Not: true}}
		}, true},
		{"remove removable", "menu.greeting", func(l *texts.MsgLayout) { l.Rows[5] = l.Rows[5][:1] }, true},
		{"remove locked buy", "menu.greeting", func(l *texts.MsgLayout) { l.Rows = slices.Delete(l.Rows, 0, 1) }, false},
		{"locked changes action", "menu.greeting", func(l *texts.MsgLayout) { l.Rows[0][0].Action = "route:info" }, false},
		{"locked gets condition", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[0][0].Conditions = append(l.Rows[0][0].Conditions, texts.CondRef{ID: "gifts_on"})
		}, false},
		{"locked label and style are free", "menu.greeting", func(l *texts.MsgLayout) {
			l.Rows[0][0].Label, l.Rows[0][0].Style = "Хочу VPN", texts.StyleDanger
		}, true},
		{"yes/no swapped positions", "payment.confirm", func(l *texts.MsgLayout) { l.Rows[0][0], l.Rows[0][1] = l.Rows[0][1], l.Rows[0][0] }, true},
		{"remove no", "payment.confirm", func(l *texts.MsgLayout) { l.Rows[0] = l.Rows[0][:1] }, false},
		{"remove have_promo", "payment.confirm", func(l *texts.MsgLayout) { l.Rows = l.Rows[:1] }, true},
		{"bound flow action elsewhere", "payment.confirm", func(l *texts.MsgLayout) {
			c := custom
			c.Action = "flow:drop_devices"
			l.Rows[1] = append(l.Rows[1], c)
		}, false},
		{"bound action moved to other id", "payment.confirm", func(l *texts.MsgLayout) { l.Rows[1][0].ID = "other" }, false},
		{"bound action twice", "payment.confirm", func(l *texts.MsgLayout) {
			c := l.Rows[0][0]
			c.ID = "yes2"
			l.Rows[1] = append(l.Rows[1], c)
		}, false},
		{"empty label without key", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Label = ""
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"label too long", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Label = strings.Repeat("я", 65)
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"label unknown var", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Label = "{{.nope}}"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, false},
		{"label with global var", "menu.greeting", func(l *texts.MsgLayout) {
			c := custom
			c.Label = "Привет {{.username}}"
			l.Rows = append(l.Rows, []texts.Cell{c})
		}, true},
		{"bad style", "menu.greeting", func(l *texts.MsgLayout) { l.Rows[2][0].Style = "red" }, false},
		{"tariffs template removed", "payment.tariffs", func(l *texts.MsgLayout) { l.Rows = l.Rows[1:] }, false},
		{"tariffs template moved", "payment.tariffs", func(l *texts.MsgLayout) { l.Rows[0], l.Rows[1] = l.Rows[1], l.Rows[0] }, true},
		{"gift template on tariffs", "payment.tariffs", func(l *texts.MsgLayout) { l.Rows[0][0].Action = "template:gift_tariffs" }, false},
		{"template with label", "payment.tariffs", func(l *texts.MsgLayout) { l.Rows[0][0].Label = "x" }, false},
		{"template shares row", "payment.tariffs", func(l *texts.MsgLayout) { l.Rows[1] = append(l.Rows[1], l.Rows[0][0]); l.Rows = l.Rows[1:] }, false},
	}
	for _, tc := range cases {
		err := valid(t, tc.key, tc.mut)
		if (err == nil) != tc.wantOK {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.wantOK)
			continue
		}
		if err != nil {
			if !errors.Is(err, texts.ErrBadMessageLayout) {
				t.Errorf("%s: not ErrBadMessageLayout: %v", tc.name, err)
			}
			if m := texts.Problem(err); m == usererr.Fallback || m == "" {
				t.Errorf("%s: no Russian message: %v", tc.name, err)
			}
		}
	}
	// сообщению без кнопок по умолчанию можно добавить свои; не-сообщению (простой текст) клавиатура не положена
	if err := texts.ValidateMessageLayout("payment.requisites", texts.MsgLayout{}, nil, true); err != nil {
		t.Errorf("empty keyboard of a message without default buttons: %v", err)
	}
	if err := texts.ValidateMessageLayout("menu.status_active", texts.MsgLayout{}, nil, true); err == nil {
		t.Error("keyboard accepted for a plain text")
	}
}

// Нестрогая проверка (загрузка в бот) не смотрит на лимиты, но не пропускает остальное.
func TestValidateRelaxed(t *testing.T) {
	def, _, _ := texts.MessageDefault("menu.greeting")
	l := def.Clone()
	for range 4 {
		l.Rows = append(l.Rows, []texts.Cell{{ID: "x" + string(rune('a'+len(l.Rows))), Label: "L", Action: "route:menu", Conditions: []texts.CondRef{}}})
	}
	if texts.ValidateMessageLayout("menu.greeting", l, nil, true) == nil {
		t.Error("strict accepted 12 rows")
	}
	if err := texts.ValidateMessageLayout("menu.greeting", l, nil, false); err != nil {
		t.Errorf("relaxed rejected 12 rows: %v", err)
	}
}

func TestCatalogs(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range texts.Conditions() {
		if c.ID == "" || c.Title == "" || c.Description == "" || seen[c.ID] {
			t.Errorf("bad condition %+v", c)
		}
		seen[c.ID] = true
	}
	for _, a := range texts.Actions() {
		if a.Title == "" || !strings.HasPrefix(a.ID, string(a.Kind)+":") {
			t.Errorf("bad action %+v", a)
		}
		if a.Guard != "" {
			if _, ok := texts.LookupCondition(a.Guard); !ok {
				t.Errorf("action %s: unknown guard %s", a.ID, a.Guard)
			}
		}
	}
	if len(texts.Templates()) != 3 {
		t.Errorf("templates = %v", texts.Templates())
	}
	for _, tpl := range texts.Templates() {
		if d, ok := texts.Lookup(tpl.Key); !ok || d.Kind != texts.KindButton {
			t.Errorf("template %s: key %q is not a registered button text", tpl.ID, tpl.Key)
		}
	}
}

func TestLegacyToLayout(t *testing.T) {
	if _, ok := texts.LegacyToLayout("menu.greeting", nil); ok {
		t.Error("nothing stored must give nothing")
	}
	// info: «Подписка» убрана, «Назад» первой
	l, ok := texts.LegacyToLayout("menu.info", map[string]texts.Layout{
		"info": {Rows: [][]string{{"button.back"}, {"button.renew"}}, Hidden: []string{"button.subscription"}},
	})
	if !ok || len(l.Rows) != 2 || l.Rows[0][0].ID != "back" || l.Rows[1][0].ID != "renew" {
		t.Fatalf("info: %+v ok=%v", l, ok)
	}
	if err := texts.ValidateMessageLayout("menu.info", l, nil, true); err != nil {
		t.Errorf("converted info invalid: %v", err)
	}
	// меню: правили только menu_new — menu остаётся по умолчанию, оба набора разведены условием
	l, ok = texts.LegacyToLayout("menu.greeting", map[string]texts.Layout{
		"menu_new": {Rows: [][]string{{"button.buy"}, {"button.support"}}, Hidden: []string{"button.trial", "button.promo", "button.gift", "button.referral"}},
	})
	if !ok {
		t.Fatal("menu not converted")
	}
	if err := texts.ValidateMessageLayout("menu.greeting", l, nil, false); err != nil {
		t.Errorf("converted menu invalid: %v", err)
	}
	var newOnly int
	for _, c := range cellsOf(l) {
		if len(c.Conditions) == 1 && c.Conditions[0].Not {
			newOnly++
		}
	}
	if newOnly != 2 { // buy + support_new
		t.Errorf("not-in-panel cells = %d, want 2", newOnly)
	}
}

func TestEqualAndClone(t *testing.T) {
	def, _, _ := texts.MessageDefault("menu.greeting")
	c := def.Clone()
	if !def.Equal(c) {
		t.Error("clone differs")
	}
	c.Rows[0][0].Conditions[0].Not = false
	if def.Equal(c) || !def.Rows[0][0].Conditions[0].Not {
		t.Error("clone shares conditions")
	}
}

// Проблемы клавиатуры показываются админу как есть: без служебной строки «Некорректное значение» перед причиной.
func TestMessageLayoutProblemIsPlain(t *testing.T) {
	for name, mut := range map[string]func(l *texts.MsgLayout){
		"guard contradiction": func(l *texts.MsgLayout) {
			l.Rows[0][1].Conditions = []texts.CondRef{{ID: "trial_available", Not: true}}
		},
		"condition twice": func(l *texts.MsgLayout) {
			l.Rows[5][0].Conditions = []texts.CondRef{{ID: "gifts_on"}, {ID: "gifts_on", Not: true}}
		},
	} {
		err := valid(t, "menu.greeting", mut)
		if err == nil {
			t.Fatalf("%s: want error", name)
		}
		if p := texts.Problem(err); p == "" || strings.Contains(p, usererr.Fallback) {
			t.Errorf("%s: Problem = %q", name, p)
		}
	}
}

// Кнопка кабинета (MiniApp) служебная: без неё на экране «Устройства» некуда вести юзера из бота в Mini App.
func TestMiniAppButtonIsLocked(t *testing.T) {
	_, locked, ok := texts.MessageDefault("devices.menu")
	if !ok || !slices.Contains(locked, "devices_miniapp") {
		t.Fatalf("devices_miniapp must be locked, locked = %v", locked)
	}
}

// Mini App включается только адресом из env (MINIAPP_ORIGIN): отдельного условия «для превью» в редакторе и схеме нет,
// кнопка просто не строится ботом, пока адреса нет.
func TestMiniAppHasNoPreviewCondition(t *testing.T) {
	for _, c := range texts.Conditions() {
		if c.ID == "miniapp_on" {
			t.Fatal("miniapp_on must not be a preview condition")
		}
	}
	if a, ok := texts.LookupAction("link:miniapp"); !ok || a.Guard != "" {
		t.Fatalf("link:miniapp must exist without a guard: %+v ok=%v", a, ok)
	}
}
