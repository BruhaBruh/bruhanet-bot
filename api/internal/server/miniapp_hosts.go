package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	hostLinkPerMin = 40
	hostLinkBody   = 1 << 10
	hostNameMax    = 64
)

var hostIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

// hostID — устойчивый короткий id ключа: по нему клиент просит ссылку, не зная её. Хеш необратим, секрет не раскрывает.
func hostID(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:8])
}

// hostName — название ключа из фрагмента ссылки (после #); нет — «Ключ N».
func hostName(link string, n int) string {
	_, frag, _ := strings.Cut(link, "#")
	name, err := url.QueryUnescape(frag)
	if err != nil {
		name = frag
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if r := []rune(name); len(r) > hostNameMax {
		name = string(r[:hostNameMax])
	}
	if name == "" {
		return fmt.Sprintf("Ключ %d", n)
	}
	return name
}

func hostProtocol(link string) string {
	scheme, _, _ := strings.Cut(link, "://")
	scheme = strings.ToLower(scheme)
	if len(scheme) > 16 || scheme == "" || strings.ContainsFunc(scheme, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '+' }) {
		return ""
	}
	return scheme
}

// minikeys — ключи подключения юзера из панели; nil, true — юзера в панели нет; ok=false — 502 уже записан.
func (s *server) miniKeys(w http.ResponseWriter, r *http.Request) (keys []string, ok bool) {
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return nil, false
	}
	if info == nil {
		return nil, true
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	keys, err := s.Panel.ConnectionKeys(ctx, info.ID)
	switch {
	case errors.Is(err, remnawave.ErrUserNotFound):
		return nil, true
	case err != nil:
		panelFailed(w, r, "panel connection keys for mini app", err)
		return nil, false
	}
	return keys, true
}

// miniHosts — список ключей подключения (хостов) без самих ссылок: id, название и протокол.
func (s *server) miniHosts(w http.ResponseWriter, r *http.Request) {
	keys, ok := s.miniKeys(w, r)
	if !ok {
		return
	}
	type item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Protocol string `json:"protocol"`
	}
	items := make([]item, 0, len(keys))
	for i, k := range keys {
		items = append(items, item{ID: hostID(k), Name: hostName(k, i+1), Protocol: hostProtocol(k)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// miniHostLink — ссылка одного ключа (доступ к VPN): только POST по нажатию «Скопировать»/«QR», не кэшируется, не логируется.
func (s *server) miniHostLink(w http.ResponseWriter, r *http.Request) {
	u := miniUser(r.Context())
	if !s.mini.allow(limitKey("host-link", u.ID), hostLinkPerMin, time.Minute, s.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, hostLinkBody)).Decode(&in); err != nil || !hostIDRe.MatchString(in.ID) {
		writeBadRequest(w)
		return
	}
	keys, ok := s.miniKeys(w, r)
	if !ok {
		return
	}
	for _, k := range keys {
		if hostID(k) == in.ID {
			stlog.L(r.Context()).Info("mini app host link issued")
			writeJSON(w, http.StatusOK, map[string]string{"url": k})
			return
		}
	}
	writeError(w, http.StatusNotFound, "not found")
}
