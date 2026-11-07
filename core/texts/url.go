package texts

import (
	"net/url"
	"strconv"
	"strings"
)

// ValidHTTPSURL — ссылка кнопки: только https с хостом, без пробелов и управляющих символов, без userinfo и opaque
// (javascript:, data:, http:, tg: отклоняются), не длиннее max байт.
func ValidHTTPSURL(raw string, max int) bool {
	if raw == "" || len(raw) > max || strings.ContainsFunc(raw, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return false
	}
	// Telegram отвечает 400 на сообщение с кнопкой, у которой порт вне диапазона: это бы потеряло всё сообщение.
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		return err == nil && n >= 1 && n <= 65535
	}
	return true
}
