package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Формат конфига страницы подписки Remnawave (платформы → приложения → шаги, тексты по локалям).
const sampleSubpage = `{
 "platforms": {
  "android": {"displayName": {"en": "Android", "ru": "Андроид"}, "apps": [
   {"name": "Happ", "featured": true, "urlScheme": "happ://add/", "isNeedBase64Encoding": false,
    "installationStep": {"description": {"en": "Install", "ru": "Установи <b>Happ</b>"}, "buttons": [
      {"buttonLink": "https://play.google.com/store/apps/details?id=x", "buttonText": {"en": "Google Play", "ru": "Google Play"}},
      {"buttonLink": "javascript:alert(1)", "buttonText": {"en": "evil"}},
      {"buttonLink": "data:text/html;base64,AAAA", "buttonText": {"en": "evil2"}},
      {"buttonLink": "file:///etc/passwd", "buttonText": {"en": "evil3"}}]},
    "addSubscriptionStep": {"description": {"en": "Add the link {{SUBSCRIPTION_LINK}}"}},
    "connectAndUseStep": {"description": {"ru": "Нажми «Подключить»"}},
    "secretField": "must not leak"},
   {"name": "Bad", "urlScheme": "javascript://x"}]},
  "ios": {"apps": [{"name": "V2Box", "urlScheme": "v2box://install-sub?url=", "isNeedBase64Encoding": true}]}
 }
}`

func TestSanitizeInstructions(t *testing.T) {
	dto, err := sanitizeInstructions([]byte(sampleSubpage))
	if err != nil {
		t.Fatal(err)
	}
	if len(dto.Platforms) != 2 || dto.Platforms[0].ID != "ios" || dto.Platforms[1].ID != "android" {
		t.Fatalf("platform order/ids: %+v", dto.Platforms)
	}
	and := dto.Platforms[1]
	if and.Title != "Андроид" || len(and.Apps) != 2 {
		t.Fatalf("android = %+v", and)
	}
	happ := and.Apps[0]
	if happ.Name != "Happ" || !happ.Featured || happ.URLScheme != "happ://add/" || happ.NeedBase64 {
		t.Fatalf("happ = %+v", happ)
	}
	if len(happ.Steps) != 3 || happ.Steps[0].Text != "Установи <b>Happ</b>" || !strings.Contains(happ.Steps[1].Text, "{{SUBSCRIPTION_LINK}}") {
		t.Fatalf("steps = %+v", happ.Steps)
	}
	if b := happ.Steps[0].Buttons; len(b) != 1 || b[0].URL != "https://play.google.com/store/apps/details?id=x" || b[0].Text != "Google Play" {
		t.Fatalf("hostile button links must be dropped, got %+v", b)
	}
	if and.Apps[1].URLScheme != "" {
		t.Fatalf("javascript scheme must be dropped: %+v", and.Apps[1])
	}
	if ios := dto.Platforms[0].Apps[0]; !ios.NeedBase64 || ios.URLScheme != "v2box://install-sub?url=" {
		t.Fatalf("ios app = %+v", ios)
	}
	if b, _ := json.Marshal(dto); strings.Contains(strings.ToLower(string(b)), "must not leak") {
		t.Fatal("unknown fields leaked into the DTO")
	}
}

func TestSanitizeInstructionsGarbage(t *testing.T) {
	for _, raw := range []string{``, `null`, `[]`, `{"platforms": 5}`, `{"platforms": {"ios": "x"}}`} {
		dto, err := sanitizeInstructions([]byte(raw))
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
