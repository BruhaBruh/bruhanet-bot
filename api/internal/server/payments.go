package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// moscow — Europe/Moscow; без DST, так что фиксированный сдвиг — безопасный запасной вариант.
var moscow = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Moscow"); err == nil {
		return l
	}
	return time.FixedZone("MSK", 3*3600)
}()

func (s *server) payments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.PaymentFilter{Status: model.PaymentStatus(q.Get("status")), Limit: pageSize}
	switch f.Status {
	case "", model.PaymentPending, model.PaymentConfirmed, model.PaymentDeclined, model.PaymentCancelled:
	default:
		writeBadRequest(w)
		return
	}

	// Дни по МСК, to включительно; по умолчанию последние 30 дней (сегодня и 29 предыдущих).
	today := s.Now().In(moscow)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, moscow)
	from, to := today.AddDate(0, 0, -29), today
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.ParseInLocation(time.DateOnly, v, moscow); err != nil {
			writeBadRequest(w)
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.ParseInLocation(time.DateOnly, v, moscow); err != nil {
			writeBadRequest(w)
			return
		}
	}
	if to.Before(from) {
		writeBadRequest(w)
		return
	}
	f.From, f.To = from, to.AddDate(0, 0, 1)

	page, ok := pageParam(q.Get("page"))
	if !ok {
		writeBadRequest(w)
		return
	}
	f.Offset = (page - 1) * pageSize

	items, total, err := s.Store.ListPayments(r.Context(), f)
	var revenue int
	if err == nil {
		revenue, err = s.Store.Revenue(r.Context(), f.From, f.To)
	}
	if err != nil {
		stlog.L(r.Context()).Error("list payments", slog.Any("error", err))
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "revenue": revenue})
}
