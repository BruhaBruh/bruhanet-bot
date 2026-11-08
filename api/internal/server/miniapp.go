package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	miniInitDataMaxAge = 2 * time.Hour
	miniGeneralLimit   = 120 // запросов в минуту на юзера
)

type miniUserKey struct{}

// miniUser — юзер, прошедший miniAuth; вне цепочки miniAuth пуст.
func miniUser(ctx context.Context) tgUser {
	u, _ := ctx.Value(miniUserKey{}).(tgUser)
	return u
}

// NewMiniApp — HTTP-обработчик Mini App: отдельный mux, свой CSP, без маршрутов админки.
func NewMiniApp(d Deps) http.Handler {
	if d.PanelTimeout == 0 {
		d.PanelTimeout = 5 * time.Second
	}
	d.Static = d.MiniStatic
	s := &server{Deps: d, secure: true, avatars: newAvatarCache(), mini: newLimiter(), instr: newInstrCache()}

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /m/api/me", s.miniMe)
	apiMux.HandleFunc("GET /m/api/subscription", s.miniSubscription)
	apiMux.HandleFunc("POST /m/api/admin/login-link", s.miniAdminLoginLink)
	apiMux.HandleFunc("POST /m/api/subscription-link", s.miniSubscriptionLink)
	apiMux.HandleFunc("GET /m/api/instructions", s.miniInstructions)
	apiMux.HandleFunc("GET /m/api/devices", s.miniDevices)
	apiMux.HandleFunc("DELETE /m/api/devices/{hwid}", s.miniDeleteDevice)
	apiMux.HandleFunc("DELETE /m/api/devices", s.miniDeleteAllDevices)
	apiMux.HandleFunc("GET /m/api/hosts", s.miniHosts)
	apiMux.HandleFunc("POST /m/api/hosts/link", s.miniHostLink)
	apiMux.HandleFunc("GET /m/api/referral", s.miniReferral)
	apiMux.HandleFunc("GET /m/api/promos", s.miniPromos)
	apiMux.HandleFunc("POST /m/api/promos/activate", s.miniActivatePromo)
	apiMux.HandleFunc("/m/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	mux := http.NewServeMux()
	mux.Handle("/m/api/", noStore(s.miniMutationGuard(s.miniAuth(apiMux))))
	mux.HandleFunc("/", s.static)
	return metrics(securityHeaders(miniCSP(d.Static), false, mux))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// miniMutationGuard: мутации только с application/json и Origin == MiniOrigin (cookie нет, это второй слой от CSRF).
func (s *server) miniMutationGuard(next http.Handler) http.Handler {
	return jsonOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if r.Header.Get("Origin") != s.MiniOrigin {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		next.ServeHTTP(w, r)
	}))
}

// miniAuth проверяет Authorization: tma <initData>, находит юзера в users и ограничивает частоту запросов.
func (s *server) miniAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, raw, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || scheme != "tma" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		u, err := parseInitData(raw, s.BotToken, s.Now(), miniInitDataMaxAge)
		switch {
		case errors.Is(err, errInitDataExpired):
			writeError(w, http.StatusUnauthorized, "session_expired")
			return
		case err != nil:
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		switch _, err = s.Store.UserByID(r.Context(), u.ID); {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusForbidden, "not_registered")
			return
		case err != nil:
			internal(w, r, "mini app user", err)
			return
		}
		if !s.mini.allow(limitKey("all", u.ID), miniGeneralLimit, time.Minute, s.Now()) {
			writeError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
		ctx := stlog.With(context.WithValue(r.Context(), miniUserKey{}, u), "telegram_id", u.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func limitKey(kind string, id int64) string { return kind + ":" + strconv.FormatInt(id, 10) }

// miniMe — имя юзера и бота для шапки; blocked нужен фронту только чтобы не обещать промокод.
func (s *server) miniMe(w http.ResponseWriter, r *http.Request) {
	tg := miniUser(r.Context())
	u, err := s.Store.UserByID(r.Context(), tg.ID)
	if err != nil {
		internal(w, r, "mini app me", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": u.Name, "username": u.Username, "botUsername": s.botWho(r.Context()).Username, "blocked": u.BlockedAt != nil,
		"isAdmin": tg.ID == s.AdminID,
	})
}
