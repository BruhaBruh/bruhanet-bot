package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
)

const generateAttempts = 5

// promoItem — промокод + username персонального юзера (из users; null, если юзера нет в таблице).
type promoItem struct {
	model.PromoWithUsage
	Username *string `json:"username"`
}

func (s *server) promoItem(r *http.Request, p model.PromoWithUsage) (promoItem, error) {
	it := promoItem{PromoWithUsage: p}
	if p.TelegramID == nil {
		return it, nil
	}
	switch u, err := s.Store.UserByID(r.Context(), *p.TelegramID); {
	case err == nil:
		it.Username = &u.Username
	case !errors.Is(err, store.ErrNotFound):
		return it, err
	}
	return it, nil
}

func (s *server) promos(w http.ResponseWriter, r *http.Request) {
	var activeOnly bool
	switch r.URL.Query().Get("status") {
	case "active":
		activeOnly = true
	case "", "all":
	default:
		writeBadRequest(w)
		return
	}
	list, err := s.Store.ListPromos(r.Context(), activeOnly, s.Now())
	if err != nil {
		internal(w, r, "list promos", err)
		return
	}
	items := make([]promoItem, 0, len(list))
	for _, p := range list {
		it, err := s.promoItem(r, p)
		if err != nil {
			internal(w, r, "promo user", err)
			return
		}
		items = append(items, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type promoInput struct {
	Code           string          `json:"code"`
	Kind           model.PromoKind `json:"kind"`
	Value          int             `json:"value"`
	MaxActivations *int            `json:"maxActivations"`
	ExpiresAt      *string         `json:"expiresAt"`
	OnlyNew        bool            `json:"onlyNew"`
	User           string          `json:"user"`
}

func (s *server) createPromo(w http.ResponseWriter, r *http.Request) {
	var in promoInput
	if !readJSONMax(w, r, &in, 4<<10, false) {
		return
	}
	p := model.Promo{Code: in.Code, Kind: in.Kind, Value: in.Value, MaxActivations: in.MaxActivations, OnlyNew: in.OnlyNew}
	if p.MaxActivations != nil && *p.MaxActivations < 1 {
		writeInvalid(w, "лимит активаций — от 1")
		return
	}
	if in.ExpiresAt != nil {
		d, err := time.ParseInLocation(time.DateOnly, *in.ExpiresAt, moscow)
		if err != nil {
			writeInvalid(w, "дата — в формате ГГГГ-ММ-ДД")
			return
		}
		end := d.AddDate(0, 0, 1) // до конца дня МСК = следующая полночь
		if !end.After(s.Now()) {
			writeInvalid(w, "дата уже прошла")
			return
		}
		p.ExpiresAt = &end
	}
	if u := strings.TrimSpace(in.User); u != "" {
		id, err := strconv.ParseInt(u, 10, 64)
		if err != nil {
			usr, err := s.Store.UserByUsername(r.Context(), u)
			if errors.Is(err, store.ErrNotFound) {
				writeInvalid(w, "юзер ещё не писал боту, укажи telegram ID")
				return
			}
			if err != nil {
				internal(w, r, "promo user", err)
				return
			}
			id = usr.TelegramID
		}
		if id <= 0 {
			writeInvalid(w, "некорректный telegram ID")
			return
		}
		p.TelegramID = &id
	}

	generated := strings.TrimSpace(in.Code) == ""
	attempts := 1
	if generated {
		attempts = generateAttempts
	}
	var created model.Promo
	var err error
	for range attempts {
		if generated {
			p.Code = promo.Generate()
		}
		if err = promo.ValidateNew(p); err != nil {
			writeInvalid(w, promoReason(err))
			return
		}
		if created, err = s.Store.CreatePromo(r.Context(), p); !errors.Is(err, store.ErrPromoExists) {
			break
		}
	}
	switch {
	case errors.Is(err, store.ErrPromoExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "exists", "message": "Такой код уже есть"})
		return
	case err != nil:
		internal(w, r, "create promo", err)
		return
	}
	it, err := s.promoItem(r, model.PromoWithUsage{Promo: created})
	if err != nil {
		internal(w, r, "promo user", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"promo": it})
}

// promoReason — человекочитаемая причина из errors.Join(ErrInvalid, причина).
func promoReason(err error) string {
	return strings.TrimPrefix(err.Error(), promo.ErrInvalid.Error()+"\n")
}

func (s *server) disablePromo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	switch err := s.Store.DisablePromo(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, r, "disable promo", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *server) promoActivations(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	if _, err := s.Store.PromoByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		internal(w, r, "promo by id", err)
		return
	}
	items, err := s.Store.PromoActivations(r.Context(), id)
	if err != nil {
		internal(w, r, "promo activations", err)
		return
	}
	if items == nil {
		items = []model.PromoActivation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
