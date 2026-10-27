package routes_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func copiesTo(e *env, chat string) int {
	n := 0
	for _, c := range e.srv.Calls("copyMessage") {
		if c.Params.Get("chat_id") == chat {
			n++
		}
	}
	return n
}

func startBroadcast(t *testing.T, e *env) {
	t.Helper()
	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Message(adminID, "Привет"))
}

func TestBroadcastFlow(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)

	e.process(t, tgtest.Message(adminID, "/a"))
	if st := e.state(t, adminID); st != "await_broadcast" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Пришлите сообщение") {
		t.Fatalf("text = %s", text)
	}

	e.process(t, tgtest.Message(adminID, "Привет"))
	if st := e.state(t, adminID); st != "confirm_broadcast" {
		t.Fatalf("state = %q", st)
	}
	p := e.last(t, "copyMessage").Params
	if p.Get("chat_id") != "1" || p.Get("from_chat_id") != "1" || p.Get("message_id") != "10" {
		t.Fatalf("preview copy = %v", p)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "*2*") {
		t.Fatalf("text = %s", text)
	}

	e.process(t, tgtest.Callback(adminID, "bc_send"))
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Рассылка запущена" {
		t.Fatalf("answer = %q", text)
	}
	for _, chat := range []string{"42", "43"} {
		var found bool
		for _, c := range e.srv.Calls("copyMessage") {
			if c.Params.Get("chat_id") == chat && c.Params.Get("from_chat_id") == "1" && c.Params.Get("message_id") == "10" {
				found = true
			}
		}
		if !found {
			t.Fatalf("no copy to %s", chat)
		}
	}
	if st := e.state(t, adminID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Разослано: *2*, не доставлено: *0*") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastDoubleTap(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.process(t, tgtest.Callback(adminID, "bc_send"))
	before := e.count("copyMessage")

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	if e.count("copyMessage") != before {
		t.Fatal("second tap sent again")
	}
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Рассылка уже отправлена или отменена" {
		t.Fatalf("answer = %q", text)
	}
}

func TestBroadcastFailedRecipient(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.srv.FailChat(43, 403, "Forbidden")

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Разослано: *1*, не доставлено: *1*") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastCancel(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Callback(adminID, "bc_cancel"))

	if st := e.state(t, adminID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Рассылка отменена") {
		t.Fatalf("text = %s", text)
	}
	e.process(t, tgtest.Message(adminID, "текст"))
	if e.count("copyMessage") != 0 {
		t.Fatal("copyMessage after cancel")
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Здравствуйте") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastAcceptsPhoto(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Photo(adminID))

	if st := e.state(t, adminID); st != "confirm_broadcast" {
		t.Fatalf("state = %q", st)
	}
	for _, c := range e.srv.Calls("sendMessage") {
		if strings.Contains(c.Params.Get("text"), "Покупка подписки") {
			t.Fatal("photo went to payment flow")
		}
	}
}

func TestBroadcastNonAdmin(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(userID, "/a"))
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Здравствуйте") {
		t.Fatalf("text = %s", text)
	}

	e.process(t, tgtest.Callback(userID, "bc_send"))
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Недостаточно прав" {
		t.Fatalf("answer = %q", text)
	}
	if e.count("copyMessage") != 0 {
		t.Fatal("copyMessage by non-admin")
	}
}

func TestBroadcastDedupesTelegramIDs(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUserAs(100, 42, now)
	e.rw.AddUserAs(101, 42, now)
	e.rw.AddUserWithoutTelegramID(102, now)

	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Message(adminID, "Привет"))
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "*1*") {
		t.Fatalf("preview = %s", text)
	}
	e.process(t, tgtest.Callback(adminID, "bc_send"))
	if n := copiesTo(e, "42"); n != 1 {
		t.Fatalf("copies to 42 = %d", n)
	}
}

func TestBroadcastPanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.rw.FailUsersAfter(1, errors.New("panel down"))

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	if copiesTo(e, "42") != 1 || copiesTo(e, "43") != 0 {
		t.Fatalf("copies: 42=%d 43=%d", copiesTo(e, "42"), copiesTo(e, "43"))
	}
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "не полностью") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastPreviewPanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.FailUsersAfter(0, errors.New("panel down"))
	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Message(adminID, "Привет"))

	if st := e.state(t, adminID); st != "await_broadcast" {
		t.Fatalf("state = %q", st)
	}
}

func TestBroadcastPreviewCopyFails(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.process(t, tgtest.Message(adminID, "/a"))
	e.srv.FailChat(adminID, 400, "Bad Request: message can't be copied")
	e.process(t, tgtest.Message(adminID, "Привет"))

	if st := e.state(t, adminID); st != "await_broadcast" {
		t.Fatalf("state = %q", st)
	}
	text := e.last(t, "sendMessage").Params.Get("text")
	if !strings.Contains(text, "Не удалось показать превью") || strings.Contains(text, "Отправить?") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastStaleCancelKeepsPayment(t *testing.T) {
	e := newEnv(t)
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, adminID), "await_photo", model.Payment{Month: 6}); err != nil {
		t.Fatal(err)
	}
	e.process(t, tgtest.Callback(adminID, "bc_cancel"))

	if st := e.state(t, adminID); st != "await_photo" {
		t.Fatalf("state = %q", st)
	}
	if p := e.payment(t, adminID); p.Month != 6 {
		t.Fatalf("payment = %+v", p)
	}
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Рассылка уже отправлена или отменена" {
		t.Fatalf("answer = %q", text)
	}
}

func TestBroadcastLogsFinished(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	e.process(t, tgtest.Message(adminID, "/a"))
	e.process(t, tgtest.Message(adminID, "Привет"))
	e.process(t, tgtest.Callback(adminID, "bc_send"))

	l := tgtest.FindLog(t, e.log, "broadcast finished")
	if l["sent"] != float64(2) || l["failed"] != float64(0) || l["interrupted"] != false || l["complete"] != true {
		t.Fatalf("entry = %v", l)
	}
}

func TestBroadcastMetrics(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.srv.FailChat(43, 403, "Forbidden")
	bc := testutil.ToFloat64(observability.Broadcasts)
	sent := testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("sent"))
	failed := testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("failed"))

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	wantDelta(t, "broadcasts", bc, testutil.ToFloat64(observability.Broadcasts), 1)
	wantDelta(t, "sent", sent, testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("sent")), 1)
	wantDelta(t, "failed", failed, testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("failed")), 1)
}
