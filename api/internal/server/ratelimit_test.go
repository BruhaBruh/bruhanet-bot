package server

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := newLimiter()
	t0 := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		if !l.allow("a", 3, time.Minute, t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("call %d must pass", i+1)
		}
	}
	if l.allow("a", 3, time.Minute, t0.Add(10*time.Second)) {
		t.Fatal("4th call inside the window must be refused")
	}
	if !l.allow("b", 3, time.Minute, t0.Add(10*time.Second)) {
		t.Fatal("keys must be independent")
	}
	if !l.allow("a", 3, time.Minute, t0.Add(61*time.Second)) {
		t.Fatal("window must free up after it passes")
	}
}
