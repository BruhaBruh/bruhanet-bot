package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Формат конфига страницы подписки Remnawave (платформы → приложения → блоки с кнопками, тексты по локалям).
const sampleSubpage = `{
 "platforms": {
  "android": {"apps": [{"name": "V2Box", "blocks": []}]},
  "ios": {"displayName": {"en": "iOS", "ru": "iOS"}, "svgIconKey": "AppleIcon", "apps": [
   {"name": "Happ", "featured": true, "svgIconKey": "Happ", "secret": "must not leak", "blocks": [
    {"title": {"en": "Install", "ru": "Установка приложения"}, "description": {"en": "Open", "ru": "Откройте <b>App Store</b>"},
     "svgIconKey": "DownloadIcon", "svgIconColor": "cyan", "buttons": [
      {"type": "external", "link": "https://apps.apple.com/x", "text": {"ru": "App Store"}},
      {"type": "external", "link": "javascript:alert(1)", "text": {"ru": "evil"}},
      {"type": "external", "link": "http://insecure.example", "text": {"ru": "http"}},
      {"type": "copyButton", "link": "https://x", "text": {"ru": "other type"}}]},
    {"title": {"ru": "Добавление подписки"}, "description": {"ru": "Нажмите кнопку"}, "svgIconKey": "CloudDownload", "svgIconColor": "teal", "buttons": [
      {"type": "subscriptionLink", "link": "happ://add/{{SUBSCRIPTION_LINK}}", "text": {"ru": "Добавить подписку"}},
      {"type": "subscriptionLink", "link": "evil://x/{{SUBSCRIPTION_LINK}}", "text": {"ru": "evil scheme"}},
      {"type": "subscriptionLink", "link": "happ://add/no-placeholder", "text": {"ru": "no placeholder"}},
      {"type": "subscriptionLink", "link": "koala-clash://install-config?url={{SUBSCRIPTION_LINK}}&name={{USERNAME}}", "text": {"ru": "Koala"}}]}
   ]}]}
 }
}`

func TestSanitizeInstructions(t *testing.T) {
	dto, err := sanitizeInstructions([]byte(sampleSubpage), "bru ha/1")
	if err != nil {
		t.Fatal(err)
	}
	if len(dto.Platforms) != 2 || dto.Platforms[0].ID != "ios" || dto.Platforms[1].ID != "android" {
		t.Fatalf("platform order/ids: %+v", dto.Platforms)
	}
	if dto.Platforms[0].Title != "iOS" || dto.Platforms[1].Title != "Android" {
		t.Fatalf("titles: %q %q", dto.Platforms[0].Title, dto.Platforms[1].Title)
	}
	happ := dto.Platforms[0].Apps[0]
	if happ.Name != "Happ" || !happ.Featured || len(happ.Blocks) != 2 {
		t.Fatalf("happ = %+v", happ)
	}
	b0 := happ.Blocks[0]
	if b0.Title != "Установка приложения" || b0.Text != "Откройте <b>App Store</b>" || b0.Icon != "download" || b0.Tone != "cyan" {
		t.Fatalf("block0 = %+v", b0)
	}
	if len(b0.Buttons) != 1 || b0.Buttons[0] != (instrButton{Text: "App Store", URL: "https://apps.apple.com/x", Kind: "link"}) {
		t.Fatalf("external buttons: hostile and non-https links must be dropped, got %+v", b0.Buttons)
	}
	b1 := happ.Blocks[1]
	if b1.Icon != "cloud" || b1.Tone != "teal" || len(b1.Buttons) != 2 {
		t.Fatalf("block1 = %+v", b1)
	}
	if b1.Buttons[0].Kind != "subscription" || b1.Buttons[0].URL != "happ://add/{{SUBSCRIPTION_LINK}}" {
		t.Fatalf("happ button = %+v", b1.Buttons[0])
	}
	// {{USERNAME}} подставляется сервером (экранированный), ссылка подписки остаётся плейсхолдером
	if got := b1.Buttons[1].URL; got != "koala-clash://install-config?url={{SUBSCRIPTION_LINK}}&name=bru+ha%2F1" {
		t.Fatalf("koala button url = %q", got)
	}
	if b, _ := json.Marshal(dto); strings.Contains(strings.ToLower(string(b)), "must not leak") {
		t.Fatal("unknown fields leaked into the DTO")
	}
}

// В subscriptionLink к схеме приложения клиент подставляет ссылку подписки: схемы только известных VPN-клиентов.
func TestSanitizeInstructionsSchemeAllowlist(t *testing.T) {
	buttons := func(link string) []instrButton {
		raw := `{"platforms":{"ios":{"apps":[{"name":"X","blocks":[{"title":{"ru":"t"},"buttons":[{"type":"subscriptionLink","link":"` + link + `","text":{"ru":"go"}}]}]}]}}}`
		dto, err := sanitizeInstructions([]byte(raw), "u")
		if err != nil || len(dto.Platforms) != 1 || len(dto.Platforms[0].Apps[0].Blocks) != 1 {
			t.Fatalf("%s: %+v %v", link, dto, err)
		}
		return dto.Platforms[0].Apps[0].Blocks[0].Buttons
	}
	for _, scheme := range []string{"happ", "v2box", "v2raytun", "v2rayng", "sing-box", "clash", "clashmeta", "hiddify", "streisand", "karing", "flclash", "flclashx", "koala-clash", "prizrak-box", "shadowrocket", "stash"} {
		link := scheme + "://x?url={{SUBSCRIPTION_LINK}}"
		if got := buttons(link); len(got) != 1 || got[0].URL != link {
			t.Errorf("%s must be allowed, got %+v", scheme, got)
		}
	}
	for _, bad := range []string{"ftp://evil.example/", "ssh://evil.example/", "intent://evil#Intent;end", "tg://resolve?domain=x", "evil://x", "HAPP://x", "https://evil.example/{x}", "javascript://x"} {
		if got := buttons(bad + "{{SUBSCRIPTION_LINK}}"); len(got) != 0 {
			t.Errorf("%s must be dropped, got %+v", bad, got)
		}
	}
}

func TestSanitizeInstructionsGarbage(t *testing.T) {
	for _, raw := range []string{``, `null`, `[]`, `{"platforms": 5}`, `{"platforms": {"ios": "x"}}`, `{"platforms":{"ios":{"apps":"x"}}}`} {
		dto, err := sanitizeInstructions([]byte(raw), "u")
		if err == nil && len(dto.Platforms) != 0 {
			t.Errorf("%q: platforms = %+v", raw, dto.Platforms)
		}
	}
}

func TestInstrCache(t *testing.T) {
	c := newInstrCache()
	t0 := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	c.put("k", instructionsDTO{Platforms: []instrPlatform{{ID: "ios"}}}, t0)
	if _, ok := c.get("k", t0.Add(9*time.Minute)); !ok {
		t.Fatal("must hit inside the TTL")
	}
	if _, ok := c.get("k", t0.Add(11*time.Minute)); ok {
		t.Fatal("must miss after the TTL")
	}
	if _, ok := c.get("other", t0); ok {
		t.Fatal("different key must miss")
	}
}
