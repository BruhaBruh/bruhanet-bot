package store

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

func TestConsumeLoginTokenOnce(t *testing.T) {
	s := testStore(t)
	h := []byte("tok-once")
	if err := s.CreateLoginToken(t.Context(), h, t0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeLoginToken(t.Context(), h, t0); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := s.ConsumeLoginToken(t.Context(), h, t0); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("second consume = %v", err)
	}
	if err := s.ConsumeLoginToken(t.Context(), []byte("unknown"), t0); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("unknown consume = %v", err)
	}
}

func TestConsumeLoginTokenExpired(t *testing.T) {
	s := testStore(t)
	h := []byte("tok-exp")
	exp := t0.Add(5 * time.Minute)
	if err := s.CreateLoginToken(t.Context(), h, exp); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeLoginToken(t.Context(), h, exp.Add(time.Second)); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired consume = %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := testStore(t)
	h := []byte("sess")
	exp := t0.Add(24 * time.Hour)
	if err := s.CreateSession(t.Context(), h, 77, exp); err != nil {
		t.Fatal(err)
	}
	id, err := s.SessionTelegramID(t.Context(), h, t0)
	if err != nil || id != 77 {
		t.Fatalf("SessionTelegramID = %d, %v", id, err)
	}
	if _, err = s.SessionTelegramID(t.Context(), h, exp.Add(time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired = %v", err)
	}
	if err = s.DeleteSession(t.Context(), h); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SessionTelegramID(t.Context(), h, t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted = %v", err)
	}
}

func TestPurgeAuth(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.CreateLoginToken(ctx, []byte("old"), t0.Add(-time.Hour)))
	must(s.CreateLoginToken(ctx, []byte("new"), t0.Add(time.Hour)))
	must(s.CreateSession(ctx, []byte("sold"), 1, t0.Add(-time.Hour)))
	must(s.CreateSession(ctx, []byte("snew"), 1, t0.Add(time.Hour)))
	must(s.PurgeAuth(ctx, t0))

	var tokens, sessions int
	must(s.pool.QueryRow(ctx, `SELECT count(*) FROM login_tokens`).Scan(&tokens))
	must(s.pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions))
	if tokens != 1 || sessions != 1 {
		t.Fatalf("tokens=%d sessions=%d, want 1/1", tokens, sessions)
	}
	if _, err := s.SessionTelegramID(ctx, []byte("snew"), t0); err != nil {
		t.Fatalf("live session purged: %v", err)
	}
}
