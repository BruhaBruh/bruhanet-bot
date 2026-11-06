package settings

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func def() Settings {
	return Defaults(map[int]int{1: 100, 3: 250}, "+7 900", "Bank", "https://t.me/sup")
}

func TestBotDefaultsMatchOldConstants(t *testing.T) {
	d := def()
	if d.ReminderTime != "12:00" || d.AdvanceLimit != 3 || d.AdvanceHours != 24 || d.ReminderWeekDays != 7 || d.ReminderDayHours != 24 {
		t.Fatalf("got %+v", d)
	}
}

func TestWithCron(t *testing.T) {
	for _, tc := range []struct {
		spec, want string
		warns      bool
	}{
		{"30 9 * * *", "09:30", false},
		{"0 0 * * *", "00:00", false},
		{"0 12,15,18 * * *", "12:00", true}, // несколько раз в день: одно время из настроек
		{"*/5 * * * *", "12:00", true},
		{"0 9 * * 1", "12:00", true},
		{"0 25 * * *", "12:00", true},
		{"60 9 * * *", "12:00", true},
		{"@daily", "12:00", true},
		{"", "12:00", true},
	} {
		got, warn := def().WithCron(tc.spec)
		if got.ReminderTime != tc.want || (warn != "") != tc.warns {
			t.Errorf("%q: time %q warn %q", tc.spec, got.ReminderTime, warn)
		}
	}
}

func TestMergeBotKeys(t *testing.T) {
	got, err := Merge(def(), map[string][]byte{
		"reminderTime": []byte(`"09:05"`), "advanceLimit": []byte(`0`), "advanceHours": []byte(`48`),
		"reminderWeekDays": []byte(`10`), "reminderDayHours": []byte(`12`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ReminderTime != "09:05" || got.AdvanceLimit != 0 || got.AdvanceHours != 48 || got.ReminderWeekDays != 10 || got.ReminderDayHours != 12 {
		t.Fatalf("got %+v", got)
	}
	if _, err := Merge(def(), map[string][]byte{"advanceLimit": []byte(`"x"`)}); !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateBotKeys(t *testing.T) {
	for name, mutate := range map[string]func(*Settings){
		"time empty":     func(s *Settings) { s.ReminderTime = "" },
		"time 24:00":     func(s *Settings) { s.ReminderTime = "24:00" },
		"time 9:00":      func(s *Settings) { s.ReminderTime = "9:00" },
		"limit negative": func(s *Settings) { s.AdvanceLimit = -1 },
		"limit big":      func(s *Settings) { s.AdvanceLimit = 101 },
		"hours zero":     func(s *Settings) { s.AdvanceHours = 0 },
		"hours big":      func(s *Settings) { s.AdvanceHours = 721 },
		"week zero":      func(s *Settings) { s.ReminderWeekDays = 0 },
		"week big":       func(s *Settings) { s.ReminderWeekDays = 61 },
		"day zero":       func(s *Settings) { s.ReminderDayHours = 0 },
		"day over week":  func(s *Settings) { s.ReminderWeekDays, s.ReminderDayHours = 1, 24 },
	} {
		s := def()
		mutate(&s)
		if err := s.Validate(); !errors.Is(err, ErrInvalidSetting) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	s := def()
	s.AdvanceLimit = 0 // 0 = авансы выключены
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
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

func TestMergeKeyNamed(t *testing.T) {
	_, err := Merge(def(), map[string][]byte{"prices": []byte(`"x"`)})
	p := Problems(err)
	if len(p) != 1 || !strings.Contains(p[0], `"prices"`) {
		t.Fatalf("problems = %v", p)
	}
	if !errors.Is(err, ErrInvalidSetting) {
		t.Fatal(err)
	}
}

func TestMergeNull(t *testing.T) {
	for _, k := range []string{"prices", "comments", "phone", "bank", "supportUrl"} {
		got, err := Merge(def(), map[string][]byte{k: []byte(" null ")})
		p := Problems(err)
		if len(p) != 1 || !strings.Contains(p[0], k) {
			t.Fatalf("%s: problems = %v", k, p)
		}
		if got.Validate() != nil {
			t.Fatalf("%s: field corrupted: %+v", k, got)
		}
	}
}

func TestValidateMessages(t *testing.T) {
	s := def()
	s.Prices = map[int]int{9: 0, 1: 0, 5: 0}
	s.Comments = []string{"ok", " ", "<b>"}
	s.Comments[2] = strings.Repeat("я", 65)
	s.Phone = strings.Repeat("1", 65)
	p := Problems(s.Validate())
	want := []string{
		"цена тарифа на 1 мес. должна быть от 1 до 1 000 000 ₽",
		"цена тарифа на 5 мес. должна быть от 1 до 1 000 000 ₽",
		"цена тарифа на 9 мес. должна быть от 1 до 1 000 000 ₽",
		"телефон должен быть от 1 до 64 символов",
		"комментарий 2 (« ») должен быть от 1 до 64 символов",
		"комментарий 3 («" + s.Comments[2] + "») должен быть от 1 до 64 символов",
	}
	if !slices.Equal(p, want) {
		t.Fatalf("problems:\n%q\nwant:\n%q", p, want)
	}
	if strings.HasPrefix(p[0], "некорректная") {
		t.Fatal("prefix leaked")
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
		{"long phone", func(s *Settings) { s.Phone = string(long) }, false},
		{"long bank", func(s *Settings) { s.Bank = string(long) }, false},
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
