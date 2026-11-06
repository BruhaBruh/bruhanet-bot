package server

import (
	"net/http"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
)

// overview — цифры карточек «Обзор» одним запросом. Окна — дни по МСК, как у выручки в /api/payments:
// неделя = сегодня и 6 предыдущих, прошлая = 7 дней до неё. Секция, которую не удалось посчитать, — null:
// остальные карточки живут (ошибка одной не роняет ответ).
func (s *server) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n := s.Now().In(moscow)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, moscow)
	end, week, prev := today.AddDate(0, 0, 1), today.AddDate(0, 0, -6), today.AddDate(0, 0, -13)

	type usersOut struct {
		Total int `json:"total"`
		New7  int `json:"new7"`
		Prev7 int `json:"prev7"`
	}
	type promosOut struct {
		Active      int `json:"active"`
		Activations int `json:"activations7"`
	}
	type revenueOut struct {
		Week int `json:"week"`
		Prev int `json:"prevWeek"`
	}
	out := struct {
		Users   *usersOut   `json:"users"`
		Promos  *promosOut  `json:"promos"`
		Pending *int        `json:"pending"`
		Revenue *revenueOut `json:"revenue"`
	}{}

	if total, nw, pv, err := s.Store.UserGrowth(ctx, prev, week, end); err != nil {
		internalLog(r, "overview users", err)
	} else {
		out.Users = &usersOut{total, nw, pv}
	}
	active, err := s.Store.ListPromos(ctx, model.PromoFilter{Status: model.PromoStatusActive}, s.Now())
	if err == nil {
		var acts int
		if acts, err = s.Store.CountPromoActivations(ctx, week, end); err == nil {
			out.Promos = &promosOut{len(active), acts}
		}
	}
	if err != nil {
		internalLog(r, "overview promos", err)
	}
	if _, total, err := s.Store.ListPayments(ctx, model.PaymentFilter{Status: model.PaymentPending, Limit: 1}); err != nil {
		internalLog(r, "overview pending", err)
	} else {
		out.Pending = &total
	}
	cur, err := s.Store.Revenue(ctx, week, end)
	if err == nil {
		var pv int
		if pv, err = s.Store.Revenue(ctx, prev, week); err == nil {
			out.Revenue = &revenueOut{cur, pv}
		}
	}
	if err != nil {
		internalLog(r, "overview revenue", err)
	}
	writeJSON(w, http.StatusOK, out)
}
