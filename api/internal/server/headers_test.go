package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCSPHashesInlineScriptsOnly(t *testing.T) {
	const inline = "\n  document.documentElement.dataset.theme = 'dark'\n"
	page := `<html><head><script>` + inline + `</script></head><body><script type="module" src="/assets/app.js"></script></body></html>`
	got := csp(fstest.MapFS{"index.html": {Data: []byte(page)}})
	sum := sha256.Sum256([]byte(inline))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(got, "script-src 'self' "+want) {
		t.Fatalf("inline script not allowed by hash: %s", got)
	}
	if strings.Count(got, "sha256-") != 1 {
		t.Fatalf("the module script with src must not be hashed: %s", got)
	}
	for _, d := range []string{"default-src 'self'", "frame-ancestors 'none'", "img-src 'self' data: blob:", "object-src 'none'"} {
		if !strings.Contains(got, d) {
			t.Errorf("missing %q in %s", d, got)
		}
	}
	// нет index.html (заглушка): остаётся только 'self', без падения
	if got := csp(fstest.MapFS{}); !strings.Contains(got, "script-src 'self';") {
		t.Fatalf("no index.html: %s", got)
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := securityHeaders("default-src 'self'", true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/anything", nil))
	for k, want := range map[string]string{
		"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer",
		"Content-Security-Policy": "default-src 'self'",
	} {
		if got := w.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if !strings.Contains(w.Header().Get("Permissions-Policy"), "camera=()") || w.Code != http.StatusTeapot {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
}
