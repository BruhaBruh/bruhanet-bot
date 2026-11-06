package server

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/bruhabruh/bruhanet/core/panelsync"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// runSync — ручная синхронизация с панелью; синхронно, потолок — panelsync.Timeout.
func (s *server) runSync(w http.ResponseWriter, r *http.Request) {
	res, err := panelsync.Run(r.Context(), s.Panel, s.Store, s.Now)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, store.ErrSyncRunning):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "sync_running", "message": "Синхронизация уже идёт"})
	case errors.Is(err, panelsync.ErrPanel):
		stlog.L(r.Context()).Warn("panel sync", slog.Any("error", err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable", "message": "Панель недоступна, данные не изменены"})
	default:
		stlog.L(r.Context()).Error("panel sync", slog.Any("error", err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "message": "Не удалось синхронизировать"})
	}
}

func (s *server) lastSync(w http.ResponseWriter, r *http.Request) {
	var at *time.Time
	switch last, err := s.Store.LastPanelSync(r.Context()); {
	case err == nil:
		at = &last.FinishedAt
	case !errors.Is(err, store.ErrNotFound):
		stlog.L(r.Context()).Error("last panel sync", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]*time.Time{"lastSyncAt": at})
}
