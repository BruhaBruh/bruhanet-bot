package broadcast

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateButtons(t *testing.T) {
	ok := Button{Text: "Открыть", URL: "https://example.com/a?b=1", Style: "primary"}
	if err := ValidateButtons([]Button{ok, ok, ok}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateButtons(nil); err != nil {
		t.Fatal(err)
	}
	bad := map[string]Button{
		"javascript":    {Text: "x", URL: "javascript:alert(1)"},
		"data":          {Text: "x", URL: "data:text/html,<script>"},
		"http":          {Text: "x", URL: "http://example.com"},
		"tg":            {Text: "x", URL: "tg://resolve?domain=x"},
		"no host":       {Text: "x", URL: "https://"},
		"opaque":        {Text: "x", URL: "https:javascript"},
		"userinfo":      {Text: "x", URL: "https://a:b@example.com"},
		"space":         {Text: "x", URL: "https://exa mple.com"},
		"newline":       {Text: "x", URL: "https://example.com/\njavascript:1"},
		"upper scheme":  {Text: "x", URL: "JAVASCRIPT:alert(1)"},
		"empty url":     {Text: "x"},
		"long url":      {Text: "x", URL: "https://e.com/" + strings.Repeat("a", MaxButtonURL)},
		"empty text":    {URL: "https://e.com"},
		"text spaces":   {Text: " x", URL: "https://e.com"},
		"vars":          {Text: "Привет {{.name}}", URL: "https://e.com"},
		"long text":     {Text: strings.Repeat("я", 65), URL: "https://e.com"},
		"emoji 2 units": {Text: strings.Repeat("😀", 33), URL: "https://e.com"},
		"style":         {Text: "x", URL: "https://e.com", Style: "red"},
	}
	for name, b := range bad {
		if err := ValidateButtons([]Button{b}); !errors.Is(err, ErrButtons) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if err := ValidateButtons([]Button{{Text: strings.Repeat("я", 64), URL: "HTTPS://Example.com"}}); err != nil {
		t.Fatalf("boundary 64 / upper-case scheme: %v", err)
	}
	if err := ValidateButtons([]Button{ok, ok, ok, ok}); !errors.Is(err, ErrButtons) {
		t.Fatalf("4 buttons: %v", err)
	}
}

func TestValidateAudience(t *testing.T) {
	d := func(n int) *int { return &n }
	for _, c := range []struct {
		a    string
		days *int
		ok   bool
	}{
		{"all", nil, true},
		{"active", nil, true},
		{"no_subscription", nil, true},
		{"expiring", d(1), true},
		{"expiring", d(60), true},
		{"expiring", nil, false},
		{"expiring", d(0), false},
		{"expiring", d(61), false},
		{"all", d(7), false},
		{"trial", nil, true},
		{"trial", d(3), false},
		{"", nil, false},
	} {
		if err := ValidateAudience(c.a, c.days); (err == nil) != c.ok {
			t.Errorf("%+v: err = %v", c, err)
		}
	}
}

func TestKeyboard(t *testing.T) {
	if Keyboard(nil) != nil {
		t.Fatal("empty must be nil")
	}
	k := Keyboard([]Button{{Text: "a", URL: "https://e.com", Style: "success"}, {Text: "b", URL: "https://e.org"}})
	if len(k.InlineKeyboard) != 2 || k.InlineKeyboard[0][0].Style != "success" || k.InlineKeyboard[1][0].URL != "https://e.org" {
		t.Fatalf("%+v", k)
	}
}
