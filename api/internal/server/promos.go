package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/core/usererr"
)

const generateAttempts = 5

// promoItem — промокод + username персонального юзера (из users; null, если юзера нет в таблице).
type promoItem struct {
	model.PromoWithUsage
	Username *string `json:"username"`
	// ValidFrom — включительная дата начала (МСК); startsAt — сырая граница (00:00 МСК).
	ValidFrom *string `json:"validFrom"`
	// ValidUntil — включительная дата (МСК) для UI; expiresAt — сырая граница (следующая полночь МСК).
	ValidUntil *string `json:"validUntil"`
}

func (s *server) promoItem(r *http.Request, p model.PromoWithUsage) (promoItem, error) {
	it := promoItem{PromoWithUsage: p}
	if p.StartsAt != nil {
		it.ValidFrom = ptr(p.StartsAt.In(moscow).Format(time.DateOnly))
	}
	if p.ExpiresAt != nil {
		it.ValidUntil = ptr(p.ExpiresAt.In(moscow).AddDate(0, 0, -1).Format(time.DateOnly))
	}
	if p.TelegramID == nil {
		return it, nil
	}
	switch u, err := s.Store.UserByID(r.Context(), *p.TelegramID); {
	case err == nil:
		if u.Username != "" {
			it.Username = &u.Username
		}
	case !errors.Is(err, store.ErrNotFound):
		return it, err
	}
	return it, nil
}

func (s *server) promos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var f model.PromoFilter
	for _, c := range []struct {
		dst  *string
		name string
		ok   []string
	}{
		{&f.Status, "status", []string{"active", "expired", "disabled"}},
		{&f.Kind, "kind", []string{"percent", "rub", "days"}},
		{&f.Target, "target", []string{"new", "user", "all"}},
	} {
		v := q.Get(c.name)
		if v == "" || (c.name == "status" && v == "all") {
			continue
		}
		if !slices.Contains(c.ok, v) {
			writeBadRequest(w)
			return
		}
		*c.dst = v
	}
	list, err := s.Store.ListPromos(r.Context(), f, s.Now())
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
	StartsAt       *string         `json:"validFrom"`
	ExpiresAt      *string         `json:"validUntil"`
	OnlyNew        bool            `json:"onlyNew"`
	User           string          `json:"user"`
}

func (s *server) createPromo(w http.ResponseWriter, r *http.Request) {
	var in promoInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			writeInvalid(w, "слишком большой запрос")
		} else {
			writeBadRequest(w)
		}
		return
	}
	p := model.Promo{Code: in.Code, Kind: in.Kind, Value: in.Value, MaxActivations: in.MaxActivations, OnlyNew: in.OnlyNew}
	if in.StartsAt != nil {
		d, err := time.ParseInLocation(time.DateOnly, *in.StartsAt, moscow)
		if err != nil {
			writeInvalid(w, "дата начала — в формате ГГГГ-ММ-ДД")
			return
		}
		p.StartsAt = &d
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
	generated := strings.TrimSpace(in.Code) == ""
	if generated {
		p.Code = promo.Generate()
	}
	u := strings.TrimSpace(in.User)
	var username string // непустой — ещё надо найти по @username (дорогой шаг, после дешёвой валидации)
	if u != "" {
		if id, err := strconv.ParseInt(u, 10, 64); err == nil {
			p.TelegramID = &id
		} else {
			username = u
			p.TelegramID = ptr(int64(1)) // заглушка для ValidateNew, настоящий id ниже
		}
	}
	if err := promo.ValidateNew(p); err != nil {
		writeInvalid(w, promoReason(err))
		return
	}
	if username != "" {
		usr, err := s.Store.UserByUsername(r.Context(), username)
		if errors.Is(err, store.ErrNotFound) {
			writeInvalid(w, "юзер ещё не писал боту, укажи telegram ID")
			return
		}
		if err != nil {
			internal(w, r, "promo user", err)
			return
		}
		p.TelegramID = &usr.TelegramID
	}

	attempts := 1
	if generated {
		attempts = generateAttempts
	}
	var created model.Promo
	var err error
	for i := range attempts {
		if i > 0 {
			p.Code = promo.Generate()
		}
		if created, err = s.Store.CreatePromo(r.Context(), p); !errors.Is(err, store.ErrPromoExists) {
			break
		}
	}
	switch {
	case errors.Is(err, store.ErrPromoExists):
		msg := "Такой код уже есть"
		if generated {
			msg = "не удалось сгенерировать код, попробуй ещё раз"
		}
		writeJSON(w, http.StatusConflict, map[string]string{"error": "exists", "message": msg})
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
func promoReason(err error) string { return usererr.Message(err) }

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

func (s *server) enablePromo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeBadRequest(w)
		return
	}
	switch err := s.Store.EnablePromo(r.Context(), id); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case err != nil:
		internal(w, r, "enable promo", err)
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
