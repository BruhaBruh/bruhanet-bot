package texts_test

import (
	"testing"

	"github.com/bruhabruh/bruhanet/core/texts"
)

func TestValidHTTPSURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://example.com":        true,
		"https://example.com:8443/x": true,
		"https://example.com:99999":  false,
		"https://example.com:0":      false,
		"https://example.com:abc":    false,
		"http://example.com":         false,
		"javascript:alert(1)":        false,
		"https://u:p@example.com":    false,
		"https://exa mple.com":       false,
		"":                           false,
	} {
		if got := texts.ValidHTTPSURL(raw, 512); got != want {
			t.Errorf("%q: got %v, want %v", raw, got, want)
		}
	}
}
