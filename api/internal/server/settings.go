package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	raw, err := s.Store.SettingsRaw(r.Context())
	if err != nil {
		internal(w, r, "settings raw", err)
		return
	}
	cur, err := settings.Merge(s.Defaults, raw)
	if err != nil { // битый ключ в БД: остальные применены, админ видит дефолт этого ключа
		stlog.L(r.Context()).Warn("broken stored settings", slog.Any("error", err))
	}
	writeJSON(w, http.StatusOK, cur)
}

func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in settings.Settings
	if !readJSON(w, r, &in) {
		return
	}
	in.Phone, in.Bank, in.SupportURL = strings.TrimSpace(in.Phone), strings.TrimSpace(in.Bank), strings.TrimSpace(in.SupportURL)
	for i, c := range in.Comments {
		in.Comments[i] = strings.TrimSpace(c)
	}
	if err := in.Validate(); err != nil {
		p := settings.Problems(err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid", "message": strings.Join(p, "; "), "problems": p})
		return
	}
	vals := map[string][]byte{}
	for k, v := range map[string]any{
		"prices": in.Prices, "phone": in.Phone, "bank": in.Bank, "comments": in.Comments, "supportUrl": in.SupportURL,
	} {
		b, err := json.Marshal(v)
		if err != nil {
			internal(w, r, "marshal setting", err)
			return
		}
		vals[k] = b
	}
	if err := s.Store.SetSettings(r.Context(), vals); err != nil {
		internal(w, r, "set settings", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
