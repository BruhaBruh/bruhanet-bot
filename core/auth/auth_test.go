package auth_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/bruhabruh/bruhanet/core/auth"
)

func TestNewSecret(t *testing.T) {
	raw, hash, err := auth.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(b) != 32 {
		t.Fatalf("raw = %q (%d bytes, err %v)", raw, len(b), err)
	}
	if len(hash) != 32 || !bytes.Equal(hash, auth.Hash(raw)) {
		t.Fatalf("hash mismatch")
	}
	raw2, _, _ := auth.NewSecret()
	if raw == raw2 {
		t.Fatal("secrets repeat")
	}
}

func TestHashBadInput(t *testing.T) {
	if len(auth.Hash("%%%")) != 32 {
		t.Fatal("Hash of garbage must still return a digest")
	}
}
