// Package auth — генерация и хеширование секретов входа и сессий.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewSecret — 32 случайных байта: raw (base64url) для URL/cookie и sha256 для хранения.
func NewSecret() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(b), sum[:], nil
}

// Hash — хеш для поиска по raw из URL/cookie; мусорный raw даёт хеш, которого нет в базе.
func Hash(raw string) []byte {
	b, _ := base64.RawURLEncoding.DecodeString(raw)
	sum := sha256.Sum256(b)
	return sum[:]
}
