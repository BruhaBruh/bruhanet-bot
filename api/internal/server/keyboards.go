package server

import (
	"net/http"
	"slices"

	"github.com/bruhabruh/bruhanet/core/texts"
)

type kbSlot struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // button | dynamic
	Label     string `json:"label"`
	Removable bool   `json:"removable"`
	Style     string `json:"style"`
}

type kbDynamic struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// kbItem — клавиатура под сообщением, как её рисует бот: rows — действующая раскладка (сохранённая или по умолчанию),
// hidden — скрытые кнопки. Подписи и стили берутся из тех же правок текстов, что применяет бот.
type kbItem struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Messages   []string    `json:"messages"` // сообщения, под которыми бот показывает эту клавиатуру
	Slots      []kbSlot    `json:"slots"`
	Rows       [][]string  `json:"rows"`
	Hidden     []string    `json:"hidden"`
	Default    [][]string  `json:"default"`
	Overridden bool        `json:"overridden"`
	Dynamic    []kbDynamic `json:"dynamic"`
	// Inactive: сохранённая раскладка не проходит проверку, бот её не применяет (показана раскладка по умолчанию).
	Inactive bool   `json:"inactive"`
	Problem  string `json:"problem,omitempty"`
}

// keyboardItems — раскладки всех клавиатур; labels и styles — действующие подписи и стили кнопок (key → значение).
func keyboardItems(stored map[string]texts.Layout, over, styles map[string]string, custom map[string]string) map[string]kbItem {
	label := func(key string) string {
		d, ok := texts.Lookup(key)
		if !ok {
			return key
		}
		body := d.Default
		if b, ok := over[key]; ok && texts.ValidateStored(d, b, custom) == nil {
			body = b
		}
		return texts.RenderDef(d, body, texts.Samples(d, custom))
	}
	out := map[string]kbItem{}
	for _, k := range texts.Keyboards() {
		it := kbItem{ID: k.ID, Title: k.Title, Messages: k.Messages, Default: k.Default, Dynamic: []kbDynamic{}}
		for _, id := range k.Order() {
			if s, ok := slotByID(k, id); ok {
				d, _ := texts.Lookup(id)
				style := d.Style
				if st, ok := styles[id]; ok {
					style = st
				}
				it.Slots = append(it.Slots, kbSlot{ID: id, Kind: "button", Label: label(id), Removable: s.Removable, Style: style})
			}
		}
		for _, b := range k.Blocks {
			it.Dynamic = append(it.Dynamic, kbDynamic{ID: b.ID, Title: b.Title})
			it.Slots = append(it.Slots, kbSlot{ID: b.ID, Kind: "dynamic", Label: b.Title})
		}
		var layout *texts.Layout
		if l, ok := stored[k.ID]; ok {
			it.Overridden = true
			if err := k.ValidateLayout(l); err != nil {
				it.Inactive, it.Problem = true, texts.Problem(err)
			} else {
				layout = &l
			}
		}
		eff := k.Effective(layout)
		it.Rows, it.Hidden = eff.Rows, eff.Hidden
		out[k.ID] = it
	}
	return out
}

func slotByID(k texts.Keyboard, id string) (texts.Slot, bool) {
	i := slices.IndexFunc(k.Slots, func(s texts.Slot) bool { return s.ID == id })
	if i < 0 {
		return texts.Slot{}, false
	}
	return k.Slots[i], true
}

// keyboardsFor — клавиатуры сообщения key (по порядку реестра раскладок).
func keyboardsFor(items map[string]kbItem, key string) []kbItem {
	ks := texts.KeyboardsFor(key)
	out := make([]kbItem, 0, len(ks))
	for _, k := range ks {
		out = append(out, items[k.ID])
	}
	return out
}

func (s *server) putKeyboard(w http.ResponseWriter, r *http.Request) {
	k, ok := texts.LookupKeyboard(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Rows   [][]string `json:"rows"`
		Hidden []string   `json:"hidden"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l := texts.Layout{Rows: in.Rows, Hidden: in.Hidden}
	if err := k.ValidateLayout(l); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	// храним канонический вид: в рядах только видимые кнопки
	l.Rows = visibleRows(l)
	// расположение по умолчанию не храним: код сможет его менять, пока админ ничего не переставлял
	if slices.EqualFunc(k.Effective(&l).Rows, k.Default, slices.Equal) && len(l.Hidden) == 0 {
		err := s.Store.DeleteKeyboardLayout(r.Context(), k.ID)
		if err != nil {
			internal(w, r, "reset keyboard layout", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.Store.SaveKeyboardLayout(r.Context(), k.ID, l); err != nil {
		internal(w, r, "save keyboard layout", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteKeyboard(w http.ResponseWriter, r *http.Request) {
	k, ok := texts.LookupKeyboard(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Store.DeleteKeyboardLayout(r.Context(), k.ID); err != nil {
		internal(w, r, "reset keyboard layout", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func visibleRows(l texts.Layout) [][]string {
	rows := [][]string{}
	for _, row := range l.Rows {
		if r := slices.DeleteFunc(slices.Clone(row), func(id string) bool { return slices.Contains(l.Hidden, id) }); len(r) > 0 {
			rows = append(rows, r)
		}
	}
	return rows
}
