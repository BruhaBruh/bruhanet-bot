package server

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"slices"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	maxBody     = 1 << 20
	maxTextBody = 16 << 10 // тело PUT/test текста; сообщение Telegram — не больше 4096 символов
)

type textVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
	Raw         bool   `json:"raw"`
	Max         int    `json:"max"`
}

func toTextVars(vars []texts.Var) []textVar {
	out := make([]textVar, 0, len(vars))
	for _, v := range vars {
		out = append(out, textVar{Name: v.Name, Description: v.Description, Sample: v.Sample, Raw: v.Raw, Max: v.Max})
	}
	return out
}

type textItem struct {
	Key        string    `json:"key"`
	Group      string    `json:"group"`
	Title      string    `json:"title"`
	Kind       string    `json:"kind"`
	Default    string    `json:"default"`
	Body       string    `json:"body"`
	Overridden bool      `json:"overridden"`
	Vars       []textVar `json:"vars"`
	When       string    `json:"when"`
	Buttons    []string  `json:"buttons"`
	Route      string    `json:"route,omitempty"`
	Style      string    `json:"style"`
	// Keyboards: клавиатуры под сообщением с действующей раскладкой (пусто — у сообщения нет настраиваемых кнопок).
	Keyboards []kbItem `json:"keyboards"` // deprecated: общие раскладки, заменены Keyboard; уберут вместе со старым редактором
	// Keyboard: клавиатура сообщения из конструктора; null — под сообщением кнопок нет.
	Keyboard *msgKeyboard `json:"keyboard"`
	// Inactive: сохранённый текст не проходит проверку, бот его не применяет (та же texts.ValidateStored).
	Inactive bool   `json:"inactive"`
	Problem  string `json:"problem,omitempty"`
}

// readJSON читает тело в v; при ошибке сам отвечает 400.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return readJSONMax(w, r, v, maxBody, false)
}

// readJSONMax читает тело до limit байт; allowEmpty — пустое тело не ошибка.
func readJSONMax(w http.ResponseWriter, r *http.Request, v any, limit int64, allowEmpty bool) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v)
	switch {
	case err == nil, allowEmpty && errors.Is(err, io.EOF):
		return true
	case errors.As(err, new(*http.MaxBytesError)):
		writeInvalid(w, "слишком длинный текст")
	default:
		writeBadRequest(w)
	}
	return false
}

func internalLog(r *http.Request, msg string, err error) {
	stlog.L(r.Context()).Error(msg, slog.Any("error", err))
}

func internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	internalLog(r, msg, err)
	writeError(w, http.StatusInternalServerError, "internal")
}

func writeInvalid(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid", "message": msg})
}

func (s *server) texts(w http.ResponseWriter, r *http.Request) {
	over, err := s.Store.TextOverrides(r.Context())
	if err != nil {
		internal(w, r, "list texts", err)
		return
	}
	styles, err := s.Store.TextStyles(r.Context())
	if err != nil {
		internal(w, r, "list texts", err)
		return
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "list texts", err)
		return
	}
	layouts, err := s.Store.KeyboardLayouts(r.Context())
	if err != nil {
		internal(w, r, "list texts", err)
		return
	}
	kbs := keyboardItems(layouts, over, styles, cur.Vars)
	msgKbs, err := s.Store.MessageKeyboards(r.Context())
	if err != nil {
		internal(w, r, "list texts", err)
		return
	}
	mk := messageKeyboards(msgKbs, over, styles, cur.Vars)
	reg := texts.Registry()
	items := make([]textItem, 0, len(reg))
	for _, d := range reg {
		it := textItem{Key: d.Key, Group: d.Group, Title: d.Title, Kind: string(d.Kind), Default: d.Default, Body: d.Default, Vars: toTextVars(d.Vars), When: d.When, Buttons: append([]string{}, d.Buttons...), Route: d.Route, Style: d.Style, Keyboards: keyboardsFor(kbs, d.Key), Keyboard: mk[d.Key]}
		if st, ok := styles[d.Key]; ok {
			it.Style = st
		}
		if b, ok := over[d.Key]; ok {
			it.Body, it.Overridden = b, true
			if err := texts.ValidateStored(d, b, cur.Vars); err != nil {
				it.Inactive, it.Problem = true, texts.Problem(err)
			}
		}
		items = append(items, it)
	}
	cat := buildCatalogs()
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "globals": toTextVars(texts.Globals(cur.Vars)), "conditions": cat.Conditions, "actions": cat.Actions, "templates": cat.Templates})
}

func (s *server) putText(w http.ResponseWriter, r *http.Request) {
	def, ok := texts.Lookup(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Body  *string `json:"body"`
		Style *string `json:"style"` // только у кнопок; нет — стиль не меняется
	}
	if !readJSONMax(w, r, &in, maxTextBody, false) {
		return
	}
	if in.Body == nil {
		writeBadRequest(w)
		return
	}
	if in.Style != nil && (def.Kind != texts.KindButton || !slices.Contains(texts.Styles, *in.Style)) {
		writeInvalid(w, "стиль можно задать только кнопке и только из списка")
		return
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings", err)
		return
	}
	if err := texts.Validate(def, *in.Body, cur.Vars); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	if *in.Body == def.Default {
		err := s.Store.ClearTextBody(r.Context(), def.Key)
		if err != nil {
			internal(w, r, "clear text", err)
			return
		}
	} else if err := s.Store.SetText(r.Context(), def.Key, *in.Body); err != nil {
		internal(w, r, "set text", err)
		return
	}
	if in.Style != nil {
		// вид по умолчанию не храним: на других экранах у кнопки остаётся их вид из кода
		style := in.Style
		if *style == def.Style {
			style = nil
		}
		if err := s.Store.SetTextStyle(r.Context(), def.Key, style); err != nil {
			internal(w, r, "set text style", err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteText(w http.ResponseWriter, r *http.Request) {
	def, ok := texts.Lookup(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err := s.Store.ResetText(r.Context(), def.Key); err != nil {
		internal(w, r, "reset text", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testText шлёт админу текст, отрендеренный на примерах; тело необязательно — иначе текущее значение.
func (s *server) testText(w http.ResponseWriter, r *http.Request) {
	def, ok := texts.Lookup(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Body *string `json:"body"`
	}
	if !readJSONMax(w, r, &in, maxTextBody, true) {
		return
	}
	body := def.Default
	if in.Body != nil {
		body = *in.Body
	} else {
		over, err := s.Store.TextOverrides(r.Context())
		if err != nil {
			internal(w, r, "text overrides", err)
			return
		}
		if b, ok := over[def.Key]; ok {
			body = b
		}
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings", err)
		return
	}
	if err := texts.Validate(def, body, cur.Vars); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	out := texts.RenderDef(def, body, texts.Samples(def, cur.Vars))
	switch def.Kind {
	case texts.KindButton:
		out = "Кнопка: " + html.EscapeString(out)
	case texts.KindPlain:
		out = "Текст: " + html.EscapeString(out)
	case texts.KindMessage:
	}
	err = s.Store.Enqueue(r.Context(), model.OutboxMessage{EventType: "text_test", ChatID: s.AdminID, Text: out, ParseMode: "HTML"})
	if err != nil {
		internal(w, r, "enqueue text test", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
