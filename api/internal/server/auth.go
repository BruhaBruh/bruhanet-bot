package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	cookieName = "bn_session"
	sessionTTL = 24 * time.Hour
)

const staleLinkPage = `<!doctype html><html lang="ru"><meta charset="utf-8"><title>BruhaNet admin</title>` +
	`<p>Ссылка устарела. Отправь боту /login ещё раз.</p></html>`

func (s *server) auth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.Now()
	if err := s.Store.ConsumeLoginToken(ctx, auth.Hash(r.URL.Query().Get("token")), now); err != nil {
		if !errors.Is(err, store.ErrTokenInvalid) {
			stlog.Error(ctx, "failed to consume login token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(staleLinkPage))
		return
	}
	raw, hash, err := auth.NewSecret()
	if err == nil {
		err = s.Store.CreateSession(ctx, hash, s.AdminID, now.Add(sessionTTL))
	}
	if err != nil {
		stlog.Error(ctx, "failed to create session", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.setCookie(w, raw, int(sessionTTL.Seconds()))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: Secure выключен только для http-BaseURL (dev)
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// requireSession пускает только живую сессию админа; остальное — 401.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		id, err := s.Store.SessionTelegramID(r.Context(), auth.Hash(c.Value), s.Now())
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusUnauthorized, "unauthorized")
		case err != nil:
			stlog.Error(r.Context(), "failed to load session", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		case id != s.AdminID:
			writeError(w, http.StatusUnauthorized, "unauthorized")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// me — сессия уже проверена requireSession, а она пускает только AdminID.
func (s *server) me(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]int64{"telegramId": s.AdminID})
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieName)
	if err == nil {
		err = s.Store.DeleteSession(r.Context(), auth.Hash(c.Value))
	}
	if err != nil {
		stlog.Error(r.Context(), "failed to delete session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}
