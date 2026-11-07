package server

import (
	"net/http"
	"slices"

	"github.com/bruhabruh/bruhanet/core/texts"
)

// Конструктор клавиатур: у сообщения (ключ текста) своя клавиатура из кнопок {id, подпись, стиль, действие, условия}.

type cellEffective struct {
	Label   string `json:"label"`   // подпись как в редакторе: своя или тело текста кнопки (с переменными)
	Preview string `json:"preview"` // она же, отрендеренная на примерах переменных
	Style   string `json:"style"`   // стиль, который увидит юзер
}

type cellOut struct {
	texts.Cell
	Locked    bool           `json:"locked"` // служебная кнопка: нельзя убрать, менять действие и условия
	Template  bool           `json:"template"`
	Effective *cellEffective `json:"effective,omitempty"` // у шаблона нет
}

type msgKeyboard struct {
	Rows       [][]cellOut `json:"rows"`    // действующая клавиатура: сохранённая или по умолчанию
	Default    [][]cellOut `json:"default"` // как строит код
	Overridden bool        `json:"overridden"`
	Locked     []string    `json:"locked"`    // id служебных кнопок и шаблонов
	Templates  []string    `json:"templates"` // шаблоны, которые можно ставить в это сообщение (те, что есть в клавиатуре по умолчанию)
	// Inactive: сохранённая клавиатура не проходит проверку, бот её не применяет (показана клавиатура по умолчанию).
	Inactive bool   `json:"inactive"`
	Problem  string `json:"problem,omitempty"`
}

type actionOut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Guard string `json:"guard,omitempty"` // встроенное условие действия
	Bound bool   `json:"bound"`           // привязано к экрану: ставится только там, где есть в клавиатуре по умолчанию
}

type conditionOut struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type templateOut struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Key   string `json:"key"` // текст кнопки реестра, которым подписан каждый элемент блока
}

type catalogs struct {
	Conditions []conditionOut `json:"conditions"`
	Actions    []actionOut    `json:"actions"`
	Templates  []templateOut  `json:"templates"`
}

func buildCatalogs() catalogs {
	c := catalogs{Conditions: []conditionOut{}, Actions: []actionOut{}, Templates: []templateOut{}}
	for _, x := range texts.Conditions() {
		c.Conditions = append(c.Conditions, conditionOut{x.ID, x.Title, x.Description})
	}
	for _, a := range texts.Actions() {
		c.Actions = append(c.Actions, actionOut{ID: a.ID, Title: a.Title, Kind: string(a.Kind), Guard: a.Guard, Bound: a.Bound})
	}
	for _, x := range texts.Templates() {
		c.Templates = append(c.Templates, templateOut{x.ID, x.Title, x.Key})
	}
	return c
}

// messageKeyboards — клавиатуры всех сообщений, у которых они есть (ключ текста → клавиатура).
func messageKeyboards(stored map[string]texts.MsgLayout, over, styles, custom map[string]string) map[string]*msgKeyboard {
	out := map[string]*msgKeyboard{}
	for _, key := range texts.MessagesWithKeyboard() {
		def, locked, _ := texts.MessageDefault(key)
		mk := &msgKeyboard{Locked: locked, Templates: []string{}}
		for _, c := range slices.Concat(def.Rows...) {
			if c.IsTemplate() {
				mk.Templates = append(mk.Templates, c.Template())
			}
		}
		if mk.Locked == nil {
			mk.Locked = []string{}
		}
		cur := def
		if l, ok := stored[key]; ok {
			mk.Overridden = true
			if err := texts.ValidateMessageLayout(key, l, custom, false); err != nil {
				mk.Inactive, mk.Problem = true, texts.Problem(err)
			} else {
				cur = l
			}
		}
		defStyle := map[string]string{}
		for _, c := range slices.Concat(def.Rows...) {
			defStyle[c.ID] = c.Style
		}
		mk.Rows = cellRows(cur, locked, defStyle, over, styles, custom)
		mk.Default = cellRows(def, locked, defStyle, over, styles, custom)
		out[key] = mk
	}
	return out
}

func cellRows(l texts.MsgLayout, locked []string, defStyle, over, styles, custom map[string]string) [][]cellOut {
	rows := make([][]cellOut, 0, len(l.Rows))
	for _, r := range l.Rows {
		row := make([]cellOut, 0, len(r))
		for _, c := range r {
			if c.Conditions == nil {
				c.Conditions = []texts.CondRef{}
			}
			o := cellOut{Cell: c, Locked: slices.Contains(locked, c.ID), Template: c.IsTemplate()}
			if !o.Template {
				o.Effective = effectiveCell(c, defStyle[c.ID], over, styles, custom)
			}
			row = append(row, o)
		}
		rows = append(rows, row)
	}
	return rows
}

func effectiveCell(c texts.Cell, defStyle string, over, styles, custom map[string]string) *cellEffective {
	e := &cellEffective{Label: c.Label, Style: c.Style}
	body := c.Label
	if d, ok := texts.Lookup(c.Key); c.Label == "" && ok {
		body = d.Default
		if b, has := over[c.Key]; has && texts.ValidateStored(d, b, custom) == nil {
			body = b
		}
		e.Label = body
	}
	plain := texts.Def{Kind: texts.KindButton}
	e.Preview = texts.RenderDef(plain, body, texts.Samples(plain, custom))
	if e.Style == "" {
		e.Style = defStyle
		if st, ok := styles[c.Key]; ok && c.Key != "" {
			e.Style = st
		}
	}
	return e
}

func (s *server) putMessageKeyboard(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	def, _, ok := texts.MessageDefault(key)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Rows [][]texts.Cell `json:"rows"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	l := texts.MsgLayout{Rows: in.Rows}
	if l.Rows == nil {
		writeBadRequest(w)
		return
	}
	for _, row := range l.Rows {
		for i := range row {
			if row[i].Conditions == nil {
				row[i].Conditions = []texts.CondRef{}
			}
		}
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings", err)
		return
	}
	if err := texts.ValidateMessageLayout(key, l, cur.Vars, true); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	// клавиатуру по умолчанию не храним: код сможет её менять, пока админ ничего не переставлял
	if l.Equal(def) {
		err = s.Store.DeleteMessageKeyboard(r.Context(), key)
	} else {
		err = s.Store.SaveMessageKeyboard(r.Context(), key, l)
	}
	if err != nil {
		internal(w, r, "save message keyboard", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteMessageKeyboard(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, _, ok := texts.MessageDefault(key); !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Store.DeleteMessageKeyboard(r.Context(), key); err != nil {
		internal(w, r, "reset message keyboard", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
