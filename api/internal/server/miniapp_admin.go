package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	adminLinkTTL     = 5 * time.Minute  // как у токена из /login
	adminLinkMaxAge  = 30 * time.Minute // подпись запуска Mini App для такой ссылки должна быть свежее общих двух часов
	adminLinkPerMin  = 5
	adminLinkUnknown = "not found"
)

// miniAdminLoginLink — кнопка «Админка» в Mini App: админу выдаётся тот же одноразовый токен входа, что и по /login
// (ссылка ADMIN_BASE_URL/auth?token=…, 5 минут), без участия бота. Всем остальным — обычный 404, как у неизвестного пути.
func (s *server) miniAdminLoginLink(w http.ResponseWriter, r *http.Request) {
	u := miniUser(r.Context())
	if u.ID != s.AdminID || s.BaseURL == "" {
		writeError(w, http.StatusNotFound, adminLinkUnknown)
		return
	}
	now := s.Now()
	if now.Sub(u.AuthDate) > adminLinkMaxAge {
		writeError(w, http.StatusUnauthorized, "session_expired")
		return
	}
	if !s.mini.allow(limitKey("admin-link", u.ID), adminLinkPerMin, time.Minute, now) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	raw, hash, err := auth.NewSecret()
	if err == nil {
		err = s.Store.CreateLoginToken(r.Context(), hash, now.Add(adminLinkTTL))
	}
	if err != nil {
		internal(w, r, "mini app admin login token", errors.Join(errors.New("create login token"), err))
		return
	}
	stlog.L(r.Context()).Info("mini app admin login link issued")
	writeJSON(w, http.StatusOK, map[string]string{"url": strings.TrimRight(s.BaseURL, "/") + "/auth?token=" + raw})
}
