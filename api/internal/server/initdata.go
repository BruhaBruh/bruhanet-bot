package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errInitDataInvalid = errors.New("invalid telegram init data")
	errInitDataExpired = errors.New("telegram init data expired")
)

// futureSkew — на сколько auth_date может опережать часы сервера.
const futureSkew = time.Minute

// tgUser — юзер из подписанного поля user в initData.
type tgUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	// AuthDate — когда Telegram подписал запуск Mini App (не из JSON user, а из поля auth_date).
	AuthDate time.Time `json:"-"`
}

// parseInitData проверяет подпись Telegram Mini App (HMAC-SHA256 токеном бота) и возраст auth_date.
// Возвращает юзера только из подписанных данных.
func parseInitData(raw, botToken string, now time.Time, maxAge time.Duration) (tgUser, error) {
	q, err := url.ParseQuery(raw)
	if err != nil || len(q) == 0 {
		return tgUser{}, errInitDataInvalid
	}
	for _, v := range q {
		if len(v) != 1 {
			return tgUser{}, errInitDataInvalid
		}
	}
	got, err := hex.DecodeString(q.Get("hash"))
	if err != nil || len(got) != sha256.Size {
		return tgUser{}, errInitDataInvalid
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + q.Get(k)
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return tgUser{}, errInitDataInvalid
	}
	sec, err := strconv.ParseInt(q.Get("auth_date"), 10, 64)
	if err != nil {
		return tgUser{}, errInitDataInvalid
	}
	switch at := time.Unix(sec, 0); {
	case at.After(now.Add(futureSkew)):
		return tgUser{}, errInitDataInvalid
	case now.Sub(at) > maxAge:
		return tgUser{}, errInitDataExpired
	}
	var u tgUser
	if err = json.Unmarshal([]byte(q.Get("user")), &u); err != nil || u.ID == 0 {
		return tgUser{}, errInitDataInvalid
	}
	u.AuthDate = time.Unix(sec, 0)
	return u, nil
}
