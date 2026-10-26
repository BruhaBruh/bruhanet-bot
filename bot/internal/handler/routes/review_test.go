package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

const adminMessageID = 5

func inReview(t *testing.T, e *env, base time.Time) {
	t.Helper()
	e.rw.AddUser(userID, base.Add(24*time.Hour))
	p := pending(base)
	p.AdminMessageID = adminMessageID
	putPayment(t, e, "in_review", p)
}

func adminPress(data string, messageID int) *models.Update {
	u := tgtest.Callback(adminID, data)
	u.CallbackQuery.Message.Message.ID = messageID
	return u
}

func TestConfirmExtendsSubscription(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)

	e.process(t, adminPress("adm_ok:42", adminMessageID))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if p := e.last(t, "editMessageReplyMarkup").Params; p.Get("message_id") != "5" {
		t.Fatalf("editMessageReplyMarkup = %v", p)
	}
	if r := e.last(t, "setMessageReaction").Params.Get("reaction"); !strings.Contains(r, "👍") {
		t.Fatalf("reaction = %s", r)
	}
	user := e.last(t, "sendMessage").Params
	if user.Get("chat_id") != "42" || !strings.Contains(user.Get("text"), "подтвердил") {
		t.Fatalf("user message = %v", user)
	}
}

func TestDeclineRestoresSubscription(t *testing.T) {
	e := newEnv(t)
	base := now.Add(3 * 24 * time.Hour)
	inReview(t, e, base)

	e.process(t, adminPress("adm_no:42", adminMessageID))

	if got := e.rw.ExpireAt(userID); !got.Equal(base) {
		t.Fatalf("expireAt = %v, want %v", got, base)
	}
	if r := e.last(t, "setMessageReaction").Params.Get("reaction"); !strings.Contains(r, "👎") {
		t.Fatalf("reaction = %s", r)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "отклонил") {
		t.Fatalf("user message = %s", text)
	}
}

func TestConfirmTwiceExtendsOnce(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)
	e.process(t, adminPress("adm_ok:42", adminMessageID))
	e.process(t, adminPress("adm_ok:42", adminMessageID))

	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("UpdateUser calls = %d, want 1", n)
	}
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Платёж устарел" {
		t.Fatalf("второе нажатие: %q", text)
	}
}

func TestStaleAdminButton(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)

	e.process(t, adminPress("adm_ok:42", 9))

	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Платёж устарел" {
		t.Fatalf("answer = %q", text)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if p := e.last(t, "editMessageReplyMarkup").Params; p.Get("message_id") != "9" {
		t.Fatalf("editMessageReplyMarkup = %v", p)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
}

func TestConfirmByNonAdmin(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)
	u := tgtest.Callback(userID, "adm_ok:42")
	u.CallbackQuery.Message.Message.ID = adminMessageID

	e.process(t, u)

	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Недостаточно прав" {
		t.Fatalf("answer = %q", text)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
}

func TestConfirmUpdateFailureKeepsReview(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)
	e.rw.FailWith(errors.New("panel is down"))

	e.process(t, adminPress("adm_ok:42", adminMessageID))

	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Не удалось обновить пользователя, попробуйте ещё раз" {
		t.Fatalf("answer = %q", text)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	if p := e.payment(t, userID); p.AdminMessageID != adminMessageID || p.Month != 6 {
		t.Fatalf("payment = %+v", p)
	}

	e.rw.FailWith(nil)
	e.process(t, adminPress("adm_ok:42", adminMessageID))
	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("повтор после ошибки: expireAt = %v, want %v", got, want)
	}
}

func TestConfirmExpiredSubscriptionCountsFromNow(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now.Add(-90*24*time.Hour))

	e.process(t, adminPress("adm_ok:42", adminMessageID))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
}

func TestDeclineExpiredSubscriptionKeepsBase(t *testing.T) {
	e := newEnv(t)
	base := now.Add(-90 * 24 * time.Hour)
	inReview(t, e, base)

	e.process(t, adminPress("adm_no:42", adminMessageID))

	if got := e.rw.ExpireAt(userID); !got.Equal(base) {
		t.Fatalf("expireAt = %v, want %v", got, base)
	}
}

func TestConfirmRestoreFailureAsksManualCheck(t *testing.T) {
	e := newEnvWithStore(t, failCAS{Storage: fsm.NewInMemory(), from: fsm.StateNone, to: "in_review"})
	inReview(t, e, now)
	e.rw.FailUpdateWith(errors.New("panel is down"))

	e.process(t, adminPress("adm_ok:42", adminMessageID))

	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Не удалось обновить пользователя, проверьте платёж вручную" {
		t.Fatalf("answer = %q", text)
	}
	tgtest.FindLog(t, e.log, "failed to restore payment for retry")
}
