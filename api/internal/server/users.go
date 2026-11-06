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

const pageSize = 50

func writeBadRequest(w http.ResponseWriter) { writeError(w, http.StatusBadRequest, "bad_request") }

func (s *server) users(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.UserFilter{Q: q.Get("q"), Tab: model.UserTab(q.Get("tab")), Now: s.Now(), Limit: pageSize}
	switch f.Tab {
	case model.UserTabAll, model.UserTabActive, model.UserTabTrial, model.UserTabBlockedBot:
	default:
		writeBadRequest(w)
		return
	}
	page, okPage := pageParam(q.Get("page"))
	f.Offset = (page - 1) * pageSize
	var okPaid, okAdv, okPeriod bool
	f.HasPayments, okPaid = boolParam(q.Get("hasPayments"))
	f.HasUnpaidAdvances, okAdv = boolParam(q.Get("hasUnpaidAdvances"))
	f.From, f.To, okPeriod = registeredPeriod(q.Get("from"), q.Get("to"))
	if !okPage || !okPaid || !okAdv || !okPeriod {
		writeBadRequest(w)
		return
	}

	// Пробного периода ещё нет (этап 8): параметр принимается, список пуст.
	items, total := []model.UserSummary{}, 0
	if f.Tab != model.UserTabTrial {
		var err error
		if items, total, err = s.Store.ListUsers(r.Context(), f); err != nil {
			stlog.L(r.Context()).Error("list users", slog.Any("error", err))
			writeError(w, http.StatusInternalServerError, "internal")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": page, "pages": max(1, (total+pageSize-1)/pageSize),
	})
}

// pageParam — номер страницы; пусто = 1.
func pageParam(v string) (int, bool) {
	if v == "" {
		return 1, true
	}
	page, err := strconv.Atoi(v)
	return page, err == nil && page >= 1 && page <= 1_000_000
}

func boolParam(v string) (value, ok bool) {
	if v == "" {
		return false, true
	}
	value, err := strconv.ParseBool(v)
	return value, err == nil
}

// registeredPeriod — даты регистрации по МСК, to включительно; пусто — без границы.
func registeredPeriod(fromS, toS string) (from, to time.Time, ok bool) {
	var err error
	if fromS != "" {
		if from, err = time.ParseInLocation(time.DateOnly, fromS, moscow); err != nil {
			return from, to, false
		}
	}
	if toS != "" {
		if to, err = time.ParseInLocation(time.DateOnly, toS, moscow); err != nil {
			return from, to, false
		}
		if !from.IsZero() && to.Before(from) {
			return from, to, false
		}
		to = to.AddDate(0, 0, 1)
	}
	return from, to, true
}

type panelInfo struct {
	ID       *int       `json:"id"` // числовой id юзера в панели; UUID этот клиент панели не отдаёт
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
	var promos []model.UserPromo
	if err == nil {
		stats, err = s.Store.UserStats(ctx, id, 0)
	}
	if err == nil {
		payments, err = s.Store.PaymentsByUser(ctx, id)
	}
	if err == nil {
		messages, err = s.Store.DirectMessages(ctx, id)
	}
	if err == nil {
		promos, err = s.Store.UserPromos(ctx, id)
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
		p.ID, p.ExpireAt, p.Status = ptr(info.ID), ptr(info.ExpireAt.UTC()), ptr(string(info.Status))
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "stats": stats, "payments": payments, "messages": messages, "promos": promos, "panel": p})
}

func ptr[T any](v T) *T { return &v }
