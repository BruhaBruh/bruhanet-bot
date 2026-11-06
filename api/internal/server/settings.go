package server

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// current — действующие настройки, как в боте (content.Provider): невалидные сохранённые — env-дефолты и список проблем.
func (s *server) current(ctx context.Context) (settings.Settings, []string, error) {
	cur, err := s.stored(ctx)
	if err != nil {
		return cur, nil, err
	}
	if err := cur.Validate(); err != nil {
		return s.Defaults, settings.Problems(err), nil
	}
	return cur, nil, nil
}

// stored — сохранённые настройки поверх дефолтов; битый ключ в БД: остальные применены, у него дефолт.
func (s *server) stored(ctx context.Context) (settings.Settings, error) {
	raw, err := s.Store.SettingsRaw(ctx)
	if err != nil {
		return settings.Settings{}, err
	}
	cur, err := settings.Merge(s.Defaults, raw)
	if err != nil {
		stlog.Warn(ctx, "broken stored settings", "error", err)
	}
	return cur, nil
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	cur, problems, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings raw", err)
		return
	}
	out := struct {
		settings.Settings
		UsingDefaults bool     `json:"usingDefaults,omitempty"`
		Problems      []string `json:"problems,omitempty"`
	}{Settings: cur, UsingDefaults: problems != nil, Problems: problems}
	writeJSON(w, http.StatusOK, out)
}

// varsInUse — проблемы «удаляемая своя переменная есть в сохранённых текстах» с ключами этих текстов.
func (s *server) varsInUse(ctx context.Context, next map[string]string) ([]string, error) {
	old, err := s.stored(ctx)
	if err != nil {
		return nil, err
	}
	over, err := s.Store.TextOverrides(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(old.Vars)) {
		if _, kept := next[name]; kept {
			continue
		}
		var keys []string
		for _, key := range slices.Sorted(maps.Keys(over)) {
			if texts.Uses(over[key], name) {
				keys = append(keys, key)
			}
		}
		if keys != nil {
			out = append(out, fmt.Sprintf("переменная %s используется в текстах: %s", name, strings.Join(keys, ", ")))
		}
	}
	return out, nil
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
	in.ReminderTimes = in.SortedReminderTimes()
	if in.Vars == nil {
		in.Vars = map[string]string{} // null в БД Merge считает битым значением
	}
	invalid := func(p []string) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid", "message": strings.Join(p, "; "), "problems": p})
	}
	if err := in.Validate(); err != nil {
		invalid(settings.Problems(err))
		return
	}
	switch used, err := s.varsInUse(r.Context(), in.Vars); {
	case err != nil:
		internal(w, r, "vars in use", err)
		return
	case used != nil:
		invalid(used)
		return
	}
	vals := map[string][]byte{}
	for k, v := range map[string]any{
		"prices": in.Prices, "phone": in.Phone, "bank": in.Bank, "comments": in.Comments, "supportUrl": in.SupportURL,
		"reminderTimes": in.ReminderTimes, "advanceLimit": in.AdvanceLimit, "advanceHours": in.AdvanceHours,
		"reminderWeekDays": in.ReminderWeekDays, "reminderDayHours": in.ReminderDayHours,
		"vars": in.Vars,
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
