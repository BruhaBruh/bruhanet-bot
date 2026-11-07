package texts

import (
	"slices"
	"strings"
	"testing"
)

func messageKeys() map[string]bool {
	out := map[string]bool{}
	for _, d := range registry {
		if d.Kind == KindMessage {
			out[d.Key] = true
		}
	}
	return out
}

// Каждая связь ведёт на существующий экран.
func TestUXMapTargetsExist(t *testing.T) {
	keys := messageKeys()
	m := BuildUXMap()
	for _, e := range m.Entries {
		for _, x := range e.Targets {
			if !keys[x.To] {
				t.Errorf("entry %s: unknown screen %q", e.ID, x.To)
			}
		}
	}
	for _, tr := range m.Transitions {
		if tr.From != "*" && !keys[tr.From] {
			t.Errorf("transition %s %s: unknown screen", tr.From, tr.Action)
		}
		for _, to := range tr.Targets {
			if !keys[to] {
				t.Errorf("transition %s %s: unknown screen %q", tr.From, tr.Action, to)
			}
		}
	}
	for _, l := range m.Links {
		if !keys[l.From] || !keys[l.To] {
			t.Errorf("link %s -> %s: unknown screen", l.From, l.To)
		}
	}
}

// Каждая кнопка клавиатуры по умолчанию описана: ведёт на экран или заведомо наружу (ссылка, копирование).
func TestUXMapCoversDefaultKeyboards(t *testing.T) {
	for _, key := range MessagesWithKeyboard() {
		def, _, _ := MessageDefault(key)
		for _, c := range def.cells() {
			switch {
			case strings.HasPrefix(c.Action, "url:"), strings.HasPrefix(c.Action, "copy:"), strings.HasPrefix(c.Action, "link:"):
				continue
			}
			if len(MapTargets(key, c.Action)) == 0 {
				t.Errorf("%s: button %s (%s) has no transition in the UX map", key, c.ID, c.Action)
			}
		}
	}
}

// Каждый экран достижим от входов карты: новое сообщение нельзя добавить, не описав, откуда оно приходит.
func TestUXMapEveryScreenReachable(t *testing.T) {
	m := BuildUXMap()
	seen := map[string]bool{}
	var queue []string
	push := func(k string) {
		if !seen[k] {
			seen[k] = true
			queue = append(queue, k)
		}
	}
	for _, e := range m.Entries {
		for _, x := range e.Targets {
			push(x.To)
		}
	}
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		if def, _, ok := MessageDefault(k); ok {
			for _, c := range def.cells() {
				for _, to := range MapTargets(k, c.Action) {
					push(to)
				}
			}
		}
		for _, l := range m.Links {
			if l.From == k {
				push(l.To)
			}
		}
	}
	for key := range messageKeys() {
		if !seen[key] {
			t.Errorf("screen %s is not reachable in the UX map", key)
		}
	}
	// на всякий случай: порядок не важен, но таблицы не пустые
	if len(m.Entries) == 0 || !slices.ContainsFunc(m.Transitions, func(t MapTransition) bool { return t.From == "*" }) {
		t.Fatal("empty UX map tables")
	}
}
