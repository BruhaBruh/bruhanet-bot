package server

import (
	"context"
	"net/http"
)

// Устройства Mini App: логика та же, что у админки (deviceList/deleteFor), но юзер — из подписи initData, не из пути.

func (s *server) miniDevices(w http.ResponseWriter, r *http.Request) {
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	items, limit, ok := s.deviceList(w, r, info)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit})
}

func (s *server) miniDeleteDevice(w http.ResponseWriter, r *http.Request) {
	hwid := r.PathValue("hwid")
	if !validHwid(hwid) {
		writeBadRequest(w)
		return
	}
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	s.deleteFor(w, r, info, func(ctx context.Context, userID int) error { return s.Panel.DeleteUserDevice(ctx, userID, hwid) })
}

func (s *server) miniDeleteAllDevices(w http.ResponseWriter, r *http.Request) {
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	s.deleteFor(w, r, info, s.Panel.DeleteAllUserDevices)
}
