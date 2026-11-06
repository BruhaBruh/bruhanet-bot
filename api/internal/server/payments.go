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

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/go-telegram/bot"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
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

// resolveTimeout — потолок решения целиком (транзакция + вызов панели); панель ограничена PanelTimeout.
const resolveTimeout = 20 * time.Second

// panelWrite — запись срока из транзакции платежа: с потолком времени и сразу в users, как writeThrough бота.
type panelWrite struct{ s *server }

func (p panelWrite) UpdateUser(ctx context.Context, id int64, username string, expireAt time.Time, squad uuid.UUID) (*api.UserItemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.s.PanelTimeout)
	defer cancel()
	u, err := p.s.Panel.UpdateUser(ctx, id, username, expireAt, squad)
	if err != nil {
		return nil, err
	}
	// панель — источник правды: сбой записи не откатывает решение, его дочинит синхронизация
	if err := p.s.Store.SetUserPanel(ctx, id, u.ExpireAt, string(u.Status), p.s.Now()); err != nil {
		stlog.Warn(ctx, "failed to save panel state", "error", err)
	}
	return u, nil
}

// SetDeviceLimit — лимит устройств при подтверждении покупки; срок в users не меняется.
func (p panelWrite) SetDeviceLimit(ctx context.Context, id int64, limit int) (*api.UserItemInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, p.s.PanelTimeout)
	defer cancel()
	return p.s.Panel.SetDeviceLimit(ctx, id, limit)
}

// resolvePayment — подтверждение или отклонение из панели; логика та же, что у кнопок в боте (payflow.Resolve).
func (s *server) resolvePayment(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeBadRequest(w)
			return
		}
		// закрытая вкладка не должна рвать транзакцию после записи в панель
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), resolveTimeout)
		defer cancel()
		p, err := payflow.Resolve(ctx, s.Store, panelWrite{s}, payflow.Decision{
			ID: id, Approve: approve, At: s.Now(), Squad: s.Squad, AdminID: s.AdminID,
		})
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, p)
		case errors.Is(err, store.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "Платёж устарел"})
		case errors.Is(err, store.ErrNotPending):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_resolved", "message": "Уже обработано"})
		case errors.Is(err, payflow.ErrPanel):
			stlog.L(r.Context()).Error("resolve payment: panel", slog.Any("error", err))
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable", "message": "Панель не ответила, платёж не изменён"})
		default:
			internal(w, r, "resolve payment", err)
		}
	}
}
