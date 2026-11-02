package settings

import (
	"errors"
	"slices"
	"testing"
)

func def() Settings {
	return Defaults(map[int]int{1: 100, 3: 250}, "+7 900", "Bank", "https://t.me/sup")
}

func TestDefaults(t *testing.T) {
	d := def()
	if !slices.Equal(d.Comments, []string{"На кофе", "Перекус", "Одолжил", "Вернул долг"}) {
		t.Fatalf("comments: %v", d.Comments)
	}
	if d.Validate() != nil {
		t.Fatal(d.Validate())
	}
}

func TestMerge(t *testing.T) {
	raw := map[string][]byte{
		"prices":     []byte(`{"2":200}`),
		"phone":      []byte(`"+7 111"`),
		"comments":   []byte(`["x"]`),
		"supportUrl": []byte(`"https://x.io"`),
		"unknown":    []byte(`{`),
	}
	got, err := Merge(def(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prices[2] != 200 || len(got.Prices) != 1 || got.Phone != "+7 111" || got.Bank != "Bank" ||
		!slices.Equal(got.Comments, []string{"x"}) || got.SupportURL != "https://x.io" {
		t.Fatalf("got %+v", got)
	}
}

func TestMergeBroken(t *testing.T) {
	got, err := Merge(def(), map[string][]byte{"phone": []byte(`{`), "bank": []byte(`"B2"`)})
	if !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("err = %v", err)
	}
	if got.Phone != "+7 900" || got.Bank != "B2" {
		t.Fatalf("got %+v", got)
	}
}

func TestValidate(t *testing.T) {
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	tests := []struct {
		name string
		mod  func(*Settings)
		ok   bool
	}{
		{"ok", func(*Settings) {}, true},
		{"no prices", func(s *Settings) { s.Prices = nil }, false},
		{"price 0", func(s *Settings) { s.Prices = map[int]int{1: 0} }, false},
		{"price max", func(s *Settings) { s.Prices = map[int]int{1: 1_000_000} }, true},
		{"price over", func(s *Settings) { s.Prices = map[int]int{1: 1_000_001} }, false},
		{"months 0", func(s *Settings) { s.Prices = map[int]int{0: 100} }, false},
		{"months 120", func(s *Settings) { s.Prices = map[int]int{120: 100} }, true},
		{"months 121", func(s *Settings) { s.Prices = map[int]int{121: 100} }, false},
		{"empty phone", func(s *Settings) { s.Phone = " " }, false},
		{"empty bank", func(s *Settings) { s.Bank = "" }, false},
		{"no comments", func(s *Settings) { s.Comments = nil }, false},
		{"11 comments", func(s *Settings) { s.Comments = slices.Repeat([]string{"a"}, 11) }, false},
		{"empty comment", func(s *Settings) { s.Comments = []string{"a", ""} }, false},
		{"long comment", func(s *Settings) { s.Comments = []string{string(long)} }, false},
		{"bad url", func(s *Settings) { s.SupportURL = "t.me/x" }, false},
		{"ftp url", func(s *Settings) { s.SupportURL = "ftp://x.io" }, false},
		{"js url", func(s *Settings) { s.SupportURL = "javascript:alert(1)" }, false},
		{"http no host", func(s *Settings) { s.SupportURL = "https://" }, false},
		{"http url", func(s *Settings) { s.SupportURL = "http://x.io" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := def()
			tc.mod(&s)
			err := s.Validate()
			if tc.ok && err != nil {
				t.Fatal(err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalidSetting) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
