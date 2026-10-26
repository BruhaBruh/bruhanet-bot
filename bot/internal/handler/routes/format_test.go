package routes

import (
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	got := formatTime(time.Date(2026, time.January, 10, 12, 0, 0, 0, time.UTC))
	if want := "10 января 2026 г. в 15:00, MSK"; got != want {
		t.Fatalf("formatTime = %q, want %q", got, want)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[int]string{
		0:       "0 Б",
		1023:    "1023 Б",
		1536:    "1.50 КБ",
		1 << 20: "1.00 МБ",
		1 << 30: "1.00 ГБ",
	}
	for n, want := range tests {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPluralMonths(t *testing.T) {
	tests := map[int]string{
		1: "месяц", 2: "месяца", 4: "месяца", 5: "месяцев", 11: "месяцев",
		12: "месяцев", 14: "месяцев", 21: "месяц", 22: "месяца", 25: "месяцев",
	}
	for n, want := range tests {
		if got := pluralMonths(n); got != want {
			t.Errorf("pluralMonths(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPluralPeople(t *testing.T) {
	tests := map[int]string{
		1: "человек", 2: "человека", 4: "человека", 5: "человек", 11: "человек",
		12: "человек", 14: "человек", 21: "человек", 22: "человека", 25: "человек",
		101: "человек", 111: "человек",
	}
	for n, want := range tests {
		if got := pluralPeople(n); got != want {
			t.Errorf("pluralPeople(%d) = %q, want %q", n, got, want)
		}
	}
}
