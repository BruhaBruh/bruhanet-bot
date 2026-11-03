package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	usersLimit = 50
	pageSize   = 50
)

func writeBadRequest(w http.ResponseWriter) { writeError(w, http.StatusBadRequest, "bad_request") }

func (s *server) users(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.SearchUsers(r.Context(), r.URL.Query().Get("q"), usersLimit)
	if err != nil {
		stlog.L(r.Context()).Error("search users", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type panelInfo struct {
	ExpireAt *time.Time `json:"expireAt"`
	Status   *string    `json:"status"`
	Error    *string    `json:"error"`
}

func (s *server) userCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	u, err := s.Store.UserByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var stats model.UserStats
	var payments []model.Payment
	var messages []model.DirectMessage
	if err == nil {
		stats, err = s.Store.UserStats(ctx, id, 0)
	}
	if err == nil {
		payments, err = s.Store.PaymentsByUser(ctx, id)
	}
	if err == nil {
		messages, err = s.Store.DirectMessages(ctx, id)
	}
	if err != nil {
		stlog.L(ctx).Error("user card", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}

	// Панель — вспомогательные данные: её сбой не роняет карточку.
	var p panelInfo
	pctx, cancel := context.WithTimeout(ctx, s.PanelTimeout)
	defer cancel()
	switch info, err := s.Panel.GetUser(pctx, id); {
	case errors.Is(err, remnawave.ErrUserNotFound):
		p.Status = ptr("not_found")
	case err != nil:
		stlog.L(ctx).Warn("panel get user", slog.Any("error", err))
		p.Error = ptr("panel_unavailable")
	default:
		p.ExpireAt, p.Status = ptr(info.ExpireAt.UTC()), ptr(string(info.Status))
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "stats": stats, "payments": payments, "messages": messages, "panel": p})
}

func ptr[T any](v T) *T { return &v }
