package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	tmaMaxAge   = 5 * time.Minute // для входа в админку подпись должна быть свежей
	tmaBody     = 4 << 10
	tmaPerMin   = 20 // попыток в минуту на юзера из подписи (по IP нельзя: за прокси все с одного адреса)
	tmaUnauthed = "unauthorized"
)

// authTMA — вход админа одним тапом: Mini App открывается кнопкой бота, страница /tg присылает initData, сервер
// проверяет подпись (токен бота), свежесть и что это AdminID, и ставит ту же сессионную cookie, что и /auth.
// Любой отказ — один и тот же 401; подпись одноразовая; 20 попыток в минуту на юзера из подписи.
func (s *server) authTMA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("Origin") != strings.TrimRight(s.BaseURL, "/") {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var in struct {
		InitData string `json:"initData"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, tmaBody)).Decode(&in); err != nil {
		writeError(w, http.StatusUnauthorized, tmaUnauthed)
		return
	}
	// невалидная подпись ничего не тратит: мусор не должен блокировать вход
	u, err := parseInitData(in.InitData, s.BotToken, s.Now(), tmaMaxAge)
	if err != nil || s.BotToken == "" {
		writeError(w, http.StatusUnauthorized, tmaUnauthed)
		return
	}
	// лимит по юзеру из подписи: чужой юзер выбивает только свой
	if !s.tma.allow(limitKey("tma", u.ID), tmaPerMin, time.Minute, s.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	if u.ID != s.AdminID {
		writeError(w, http.StatusUnauthorized, tmaUnauthed)
		return
	}
	// подпись одноразовая: перехваченный initData второй раз не пройдёт
	q, _ := url.ParseQuery(in.InitData)
	if !s.tmaSeen.first(q.Get("hash"), tmaMaxAge+time.Minute, s.Now()) {
		writeError(w, http.StatusUnauthorized, tmaUnauthed)
		return
	}
	raw, hash, err := auth.NewSecret()
	if err == nil {
		err = s.Store.CreateSession(r.Context(), hash, s.AdminID, s.Now().Add(sessionTTL))
	}
	if err != nil {
		stlog.Error(r.Context(), "failed to create session", "error", errors.Join(errors.New("auth tma"), err))
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	s.setCookie(w, raw, int(sessionTTL.Seconds()))
	w.WriteHeader(http.StatusNoContent)
}
