package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const photoTimeout = 30 * time.Second

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

// paymentPhoto отдаёт чек платежа потоком через Bot API: токен и ссылка на файл не выходят за api.
// Тип определяется по содержимому и только картинка: заголовок Telegram не показываем.
func (s *server) paymentPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	p, err := s.Store.PaymentByID(r.Context(), id)
	switch {
	case errors.Is(err, store.ErrNotFound), err == nil && p.PhotoFileID == "":
		writeError(w, http.StatusNotFound, "not_found")
		return
	case err != nil:
		internal(w, r, "payment photo", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), photoTimeout)
	defer cancel()
	file, err := s.Telegram.OpenFile(ctx, p.PhotoFileID)
	if errors.Is(err, bot.ErrorBadRequest) { // file_id устарел
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		stlog.Warn(r.Context(), "open payment photo", "error", err)
		writeError(w, http.StatusBadGateway, "telegram_unavailable")
		return
	}
	defer func() { _ = file.Close() }()
	br := bufio.NewReader(file)
	head, _ := br.Peek(512) // короткий файл — не ошибка, битый определится ниже как не картинка
	ct := http.DetectContentType(head)
	if !slices.Contains(photoTypes, ct) {
		stlog.Warn(r.Context(), "payment photo is not an image", "content_type", ct)
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err = io.Copy(w, br); err != nil {
		stlog.Warn(r.Context(), "stream payment photo", "error", err)
	}
}
