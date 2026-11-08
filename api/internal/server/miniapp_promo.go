package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

const (
	miniPromoBody      = 4 << 10
	miniPromoPerMinute = 5
	miniPromoPerHour   = 30
)

// miniPromoUnavailable — единый ответ на любой отказ (нет кода, выключен, чужой, использован, скидочный, блокировка):
// причина наружу не раскрывается, перебор ничего не даёт.
func miniPromoUnavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "promo_unavailable", "message": "Код не найден или не подходит"})
}

// miniActivatePromo — код на бонусные дни. Скидочные коды без оплаты в Mini App бессмысленны и отвергаются тем же ответом.
func (s *server) miniActivatePromo(w http.ResponseWriter, r *http.Request) {
	tg := miniUser(r.Context())
	now := s.Now()
	if !s.mini.allow(limitKey("promo-min", tg.ID), miniPromoPerMinute, time.Minute, now) ||
		!s.mini.allow(limitKey("promo-hour", tg.ID), miniPromoPerHour, time.Hour, now) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited", "message": "Слишком часто, попробуй через минуту"})
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, miniPromoBody)).Decode(&in); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			writeError(w, http.StatusRequestEntityTooLarge, "too large")
			return
		}
		writeBadRequest(w)
		return
	}
	p, err := s.Store.PromoByCode(r.Context(), in.Code)
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, promo.ErrNotFound):
		miniPromoUnavailable(w)
		return
	case err != nil:
		internal(w, r, "mini app promo lookup", err)
		return
	}
	// закрытая вкладка не должна рвать транзакцию после записи в панель
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), resolveTimeout)
	defer cancel()
	pw := panelWrite{s}
	res, err := payflow.ActivateDaysPromo(ctx, s.Store, pw, payflow.DaysPromoRequest{
		PromoID: p.ID, TelegramID: tg.ID, Username: tg.Username, At: now, Squad: s.Squad,
		Ensure: func(ctx context.Context) (*api.UserItemInfo, error) { return pw.GetUser(ctx, tg.ID) },
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"addedDays": res.Promo.Value, "expireAt": res.NewExpire.UTC()})
	case errors.Is(err, remnawave.ErrUserNotFound):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no_subscription", "message": "Сначала оформи подписку в боте"})
	case errors.Is(err, payflow.ErrPanel):
		internalLog(r, "mini app promo: panel", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "panel_unavailable", "message": "Панель не ответила, повтори позже"})
	case isPromoRefusal(err):
		miniPromoUnavailable(w)
	default:
		internal(w, r, "mini app promo activate", err)
	}
}

func isPromoRefusal(err error) bool {
	for _, e := range []error{
		promo.ErrNotFound, promo.ErrDisabled, promo.ErrNotStarted, promo.ErrExpired, promo.ErrExhausted, promo.ErrNotYours,
		promo.ErrOnlyNew, promo.ErrAlreadyUsed, promo.ErrBlocked, promo.ErrWrongPlace, promo.ErrInvalid,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
