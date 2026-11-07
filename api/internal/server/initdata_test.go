package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testBotToken = "123456:TEST-TOKEN"

// signInitData собирает initData так же, как Telegram: data-check-string из всех полей кроме hash.
func signInitData(t *testing.T, token string, authDate time.Time, user string, extra ...string) string {
	t.Helper()
	pairs := map[string]string{"auth_date": strconv.FormatInt(authDate.Unix(), 10), "user": user, "query_id": "AAH"}
	for i := 0; i+1 < len(extra); i += 2 {
		pairs[extra[i]] = extra[i+1]
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+pairs[k])
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	v := url.Values{}
	for k, val := range pairs {
		v.Set(k, val)
	}
	v.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return v.Encode()
}

func TestParseInitData(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	const user = `{"id":42,"first_name":"Бруха 🚀","username":"bruha"}`
	valid := signInitData(t, testBotToken, now.Add(-time.Minute), user)

	tests := []struct {
		name    string
		raw     string
		wantErr error
		wantID  int64
	}{
		{"valid", valid, nil, 42},
		{"unicode name and signature field", signInitData(t, testBotToken, now, user, "signature", "abc_DEF-123"), nil, 42},
		{"wrong token", signInitData(t, "999:OTHER", now, user), errInitDataInvalid, 0},
		{"tampered user id", strings.Replace(valid, url.QueryEscape(`"id":42`), url.QueryEscape(`"id":43`), 1), errInitDataInvalid, 0},
		{"expired", signInitData(t, testBotToken, now.Add(-3*time.Hour), user), errInitDataExpired, 0},
		{"from the future", signInitData(t, testBotToken, now.Add(2*time.Minute), user), errInitDataInvalid, 0},
		{"empty", "", errInitDataInvalid, 0},
		{"no hash", "auth_date=1&user=%7B%7D", errInitDataInvalid, 0},
		{"duplicate key", valid + "&user=%7B%22id%22%3A1%7D", errInitDataInvalid, 0},
		{"user not json", signInitData(t, testBotToken, now, "nope"), errInitDataInvalid, 0},
		{"user id zero", signInitData(t, testBotToken, now, `{"id":0}`), errInitDataInvalid, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, err := parseInitData(tc.raw, testBotToken, now, 2*time.Hour)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && u.ID != tc.wantID {
				t.Fatalf("id = %d, want %d", u.ID, tc.wantID)
			}
		})
	}
	if u, _ := parseInitData(valid, testBotToken, now, 2*time.Hour); u.Username != "bruha" || u.FirstName != "Бруха 🚀" {
		t.Fatalf("user fields = %+v", u)
	}
}
