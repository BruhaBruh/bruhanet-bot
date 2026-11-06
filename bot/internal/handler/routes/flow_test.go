package routes_test

import (
	"strings"
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

	e.process(t, confirmPress(e.pending(t, userID).ID))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("после подтверждения expireAt = %v, want %v", got, want)
	}
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
}

// round — buy и фото; возвращает id платежа и выдан ли аванс (по вызовам UpdateUser и тексту юзеру).
func (e *env) round(t *testing.T) (int64, bool) {
	t.Helper()
	e.process(t, tgtest.Callback(userID, "buy:6"))
	before := len(e.rw.Updates())
	e.process(t, tgtest.Photo(userID))
	advanced := len(e.rw.Updates()) > before
	text := e.last(t, "sendMessage").Params.Get("text")
	if strings.Contains(text, "1 день") != advanced {
		t.Fatalf("аванс %v, а текст юзеру: %s", advanced, text)
	}
	return e.pending(t, userID).ID, advanced
}

func (e *env) lastReview(t *testing.T) string {
	t.Helper()
	var text string
	for _, c := range e.srv.Calls("sendMessage") {
		if c.Params.Get("chat_id") == "1" && strings.Contains(c.Params.Get("text"), "Покупка подписки") {
			text = c.Params.Get("text")
		}
	}
	return text
}

func TestAdvanceLimit(t *testing.T) {
	e := newEnv(t)
	for i := range 3 {
		id, advanced := e.round(t)
		if !advanced {
			t.Fatalf("круг %d без аванса", i+1)
		}
		e.process(t, declinePress(id))
	}

	id, advanced := e.round(t)
	if advanced {
		t.Fatal("4-е фото получило аванс")
	}
	if text := e.lastReview(t); !strings.Contains(text, "⚠️ Авансов без оплаты: 3 — без аванса") {
		t.Fatalf("review = %s", text)
	}

	e.process(t, confirmPress(id))
	if _, advanced := e.round(t); !advanced {
		t.Fatal("после подтверждения аванс не выдан")
	}
}

// Лимит и длина аванса — из настроек: правка из админки применяется без перезапуска, база для продления прежняя.
func TestAdvanceFromSettings(t *testing.T) {
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{"advanceHours": []byte(`36`), "advanceLimit": []byte(`1`)})

	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(36 * time.Hour)) {
		t.Fatalf("после фото expireAt = %v", got)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "на 36 часов") {
		t.Fatalf("юзеру: %s", text)
	}
	id := e.pending(t, userID).ID
	e.process(t, declinePress(id))
	if got := e.rw.ExpireAt(userID); !got.Equal(now) {
		t.Fatalf("после отклонения expireAt = %v, want prev %v", got, now)
	}

	before := len(e.rw.Updates())
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))
	if len(e.rw.Updates()) != before {
		t.Fatal("аванс выдан сверх лимита 1")
	}
	if text := e.lastReview(t); !strings.Contains(text, "⚠️ Авансов без оплаты: 1 — без аванса") {
		t.Fatalf("review = %s", text)
	}
}

func TestAdvanceBypassBlocked(t *testing.T) {
	e := newEnv(t)
	for i := range 3 {
		if _, advanced := e.round(t); !advanced {
			t.Fatalf("круг %d без аванса", i+1)
		}
	}
	if _, advanced := e.round(t); advanced {
		t.Fatal("аванс выдан после 3 отменённых авансов")
	}
}

func TestCancelWithoutAdvanceNotCounted(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if _, advanced := e.round(t); !advanced {
		t.Fatal("аванс не выдан")
	}
}
