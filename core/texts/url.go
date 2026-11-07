package texts

import (
	"net/url"
	"strings"
)

// ValidHTTPSURL — ссылка кнопки: только https с хостом, без пробелов и управляющих символов, без userinfo и opaque
// (javascript:, data:, http:, tg: отклоняются), не длиннее max байт.
func ValidHTTPSURL(raw string, max int) bool {
	if raw == "" || len(raw) > max || strings.ContainsFunc(raw, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}
