package server

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

// inlineScript — <script> без src: такие скрипты CSP разрешает только по хешу (тема до первой отрисовки в index.html).
var inlineScript = regexp.MustCompile(`(?s)<script([^>]*)>(.*?)</script>`)

// csp строит Content-Security-Policy по index.html: свои скрипты и стили, картинки со своего origin, data: и blob:
// (превью фото), inline-скрипты — только по sha256. Стили inline разрешены: React и @xyflow ставят style-атрибуты.
func csp(static fs.FS) string { return buildCSP(static, "", "'none'") }

// miniCSP — для Mini App: SDK Telegram с telegram.org (единственный внешний хост) и показ только во фреймах Telegram.
func miniCSP(static fs.FS) string {
	return buildCSP(static, "https://telegram.org", "https://web.telegram.org https://*.telegram.org")
}

func buildCSP(static fs.FS, extraScript, frameAncestors string) string {
	scripts := []string{"'self'"}
	if extraScript != "" {
		scripts = append(scripts, extraScript)
	}
	if b, err := fs.ReadFile(static, "index.html"); err == nil {
		for _, m := range inlineScript.FindAllSubmatch(b, -1) {
			if strings.Contains(string(m[1]), "src=") {
				continue
			}
			sum := sha256.Sum256(m[2])
			scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + strings.Join(scripts, " "),
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors " + frameAncestors,
	}, "; ")
}

// securityHeaders — заголовки на каждый ответ: админку нельзя встроить во фрейм, браузер не угадывает тип, реферер не
// уходит наружу. HSTS здесь не ставим: он живёт в nginx (приложение за прокси может слушать и http).
func securityHeaders(policy string, denyFrames bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if denyFrames {
			h.Set("X-Frame-Options", "DENY")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", policy)
		next.ServeHTTP(w, r)
	})
}
