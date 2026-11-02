package server

import (
	"encoding/json"
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const maxBody = 1 << 20

type textVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
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
}

// readJSON читает тело в v; при ошибке сам отвечает 400.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(v); err != nil {
		writeBadRequest(w)
		return false
	}
	return true
}

func internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	stlog.L(r.Context()).Error(msg, slog.Any("error", err))
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
	reg := texts.Registry()
	items := make([]textItem, 0, len(reg))
	for _, d := range reg {
		it := textItem{Key: d.Key, Group: d.Group, Title: d.Title, Kind: string(d.Kind), Default: d.Default, Body: d.Default, Vars: []textVar{}}
		if b, ok := over[d.Key]; ok {
			it.Body, it.Overridden = b, true
		}
		for _, v := range d.Vars {
			it.Vars = append(it.Vars, textVar{Name: v.Name, Description: v.Description, Sample: v.Sample})
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) putText(w http.ResponseWriter, r *http.Request) {
	def, ok := texts.Lookup(r.PathValue("key"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		Body *string `json:"body"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Body == nil {
		writeBadRequest(w)
		return
	}
	if err := texts.Validate(def, *in.Body); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	if *in.Body == def.Default {
		err := s.Store.ResetText(r.Context(), def.Key)
		if err != nil {
			internal(w, r, "reset text", err)
			return
		}
	} else if err := s.Store.SetText(r.Context(), def.Key, *in.Body); err != nil {
		internal(w, r, "set text", err)
		return
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
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeBadRequest(w)
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
	if err := texts.Validate(def, body); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	out := texts.RenderDef(def, body, texts.Samples(def))
	switch def.Kind {
	case texts.KindButton:
		out = "Кнопка: " + html.EscapeString(out)
	case texts.KindPlain:
		out = "Текст: " + html.EscapeString(out)
	case texts.KindMessage:
	}
	err := s.Store.Enqueue(r.Context(), model.OutboxMessage{EventType: "text_test", ChatID: s.AdminID, Text: out, ParseMode: "HTML"})
	if err != nil {
		internal(w, r, "enqueue text test", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
