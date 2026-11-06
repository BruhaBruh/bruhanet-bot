package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
)

// maxGiftBatch — сколько подарков можно выдать за один запрос.
const maxGiftBatch = 20

// gifts — страница подарков; status: пусто/all — все, иначе pending|ready|redeemed|cancelled. Код pending-подарка пустой.
func (s *server) gifts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := model.GiftFilter{Limit: pageSize}
	if v := q.Get("status"); v != "" && v != "all" {
		if !slices.Contains([]string{"pending", "ready", "redeemed", "cancelled"}, v) {
			writeBadRequest(w)
			return
		}
		f.Status = model.GiftStatus(v)
	}
	page, ok := pageParam(q.Get("page"))
	if !ok {
		writeBadRequest(w)
		return
	}
	f.Offset = (page - 1) * pageSize
	items, total, err := s.Store.ListGifts(r.Context(), f)
	if err != nil {
		internal(w, r, "list gifts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type giftInput struct {
	Months int `json:"months"`
	Count  int `json:"count"`
}

// createGifts выдаёт count подарков от админа на months месяцев: сразу готовые ссылки.
func (s *server) createGifts(w http.ResponseWriter, r *http.Request) {
	var in giftInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil {
		writeBadRequest(w)
		return
	}
	switch {
	case in.Months < 1 || in.Months > model.MaxGiftMonths:
		writeInvalid(w, "срок подарка — от 1 до "+strconv.Itoa(model.MaxGiftMonths)+" месяцев")
		return
	case in.Count < 1 || in.Count > maxGiftBatch:
		writeInvalid(w, "количество — от 1 до "+strconv.Itoa(maxGiftBatch))
		return
	}
	items, err := s.Store.CreateAdminGifts(r.Context(), in.Months, in.Count, s.Now())
	if err != nil {
		internal(w, r, "create gifts", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": items})
}

// cancelGift отменяет готовый подарок; активированный или уже отменённый — 409.
func (s *server) cancelGift(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	switch err := s.Store.CancelGift(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrGiftNotActive):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not_active", "message": "Подарок уже использован или отменён"})
	case err != nil:
		internal(w, r, "cancel gift", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
