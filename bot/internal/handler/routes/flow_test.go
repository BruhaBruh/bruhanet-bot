package routes_test

import (
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func TestFullPaymentFlow(t *testing.T) {
	e := newEnv(t)

	e.process(t, tgtest.Message(userID, "/start"))
	e.process(t, tgtest.Callback(userID, "pay"))
	e.process(t, tgtest.Callback(userID, "price:6"))
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))

	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("после фото expireAt = %v", got)
	}
	adminMsg := e.payment(t, userID).AdminMessageID

	e.process(t, adminPress("adm_ok:42", adminMsg))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("после подтверждения expireAt = %v, want %v", got, want)
	}
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
}
