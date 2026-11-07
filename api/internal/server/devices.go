package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const maxHwidLen = 256

type deviceItem struct {
	Hwid        string    `json:"hwid"`
	Platform    *string   `json:"platform"`
	OSVersion   *string   `json:"osVersion"`
	DeviceModel *string   `json:"deviceModel"`
	UserAgent   *string   `json:"userAgent"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func nilStr(v api.NilString) *string {
	if v.Null {
		return nil
	}
	return &v.Value
}

func validHwid(h string) bool {
	if h == "" || len(h) > maxHwidLen || !utf8.ValidString(h) {
		return false
	}
	for _, r := range h {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// panelUser находит юзера: 404 — нет в users, found=false — нет в панели, ok=false — ответ уже записан.
func (s *server) panelUser(w http.ResponseWriter, r *http.Request) (info *api.UserItemInfo, ok bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return nil, false
	}
	switch _, err := s.Store.UserByID(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
		return nil, false
	case err != nil:
		internal(w, r, "devices user", err)
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	info, err = s.Panel.GetUser(ctx, id)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return nil, true
	case err != nil:
		panelFailed(w, r, "panel get user for devices", err)
		return nil, false
	}
	return info, true
}

func panelFailed(w http.ResponseWriter, r *http.Request, msg string, err error) {
	stlog.L(r.Context()).Warn(msg, slog.Any("error", err))
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable", "message": "Панель не ответила, повторите позже"})
}

// devices — список HWID-устройств юзера из панели. Юзера нет в панели — пустой список.
func (s *server) devices(w http.ResponseWriter, r *http.Request) {
	info, ok := s.panelUser(w, r)
	if !ok {
		return
	}
	items := []deviceItem{}
	var limit *int
	if info != nil {
		ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
		defer cancel()
		list, err := s.Panel.UserDevices(ctx, info.ID)
		if err != nil && !errors.Is(err, remnawave.ErrUserNotFound) {
			panelFailed(w, r, "panel list devices", err)
			return
		}
		for _, d := range list {
			items = append(items, deviceItem{
				Hwid: d.Hwid, Platform: nilStr(d.Platform), OSVersion: nilStr(d.OsVersion),
				DeviceModel: nilStr(d.DeviceModel), UserAgent: nilStr(d.UserAgent),
				CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
			})
		}
		if l, has := remnawave.DeviceLimit(*info); has {
			limit = &l
		}
		stlog.L(r.Context()).Info("devices listed", slog.Int("count", len(items)))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "limit": limit})
}

// deleteDevice — одно устройство; панель ищет по паре (id юзера, hwid), чужое не заденет.
func (s *server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	hwid := r.PathValue("hwid")
	if !validHwid(hwid) {
		writeBadRequest(w)
		return
	}
	s.removeDevices(w, r, func(ctx context.Context, userID int) error { return s.Panel.DeleteUserDevice(ctx, userID, hwid) })
}

func (s *server) deleteAllDevices(w http.ResponseWriter, r *http.Request) {
	s.removeDevices(w, r, s.Panel.DeleteAllUserDevices)
}

func (s *server) removeDevices(w http.ResponseWriter, r *http.Request, del func(context.Context, int) error) {
	info, ok := s.panelUser(w, r)
	if !ok {
		return
	}
	if info == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	switch err := del(ctx, info.ID); {
	case errors.Is(err, remnawave.ErrDeviceNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		panelFailed(w, r, "panel delete devices", err)
	default:
		stlog.L(r.Context()).Info("devices deleted", slog.String("route", r.Pattern))
		w.WriteHeader(http.StatusNoContent)
	}
}
