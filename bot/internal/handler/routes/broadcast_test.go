package routes_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

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
	if st := e.state(t, adminID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Рассылка поставлена в очередь: *2*") {
		t.Fatalf("text = %s", text)
	}
}

func TestBroadcastQueued(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUserAs(100, 42, now)
	e.rw.AddUserAs(101, 42, now)
	e.rw.AddUser(43, now)
	e.rw.AddUserWithoutTelegramID(102, now)
	startBroadcast(t, e)
	previews := e.count("copyMessage")

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	out := e.pay.Outbox()
	if len(out) != 3 {
		t.Fatalf("outbox = %+v", out)
	}
	id := out[2].Broadcast
	if id == "" {
		t.Fatal("empty broadcast id")
	}
	chats := make([]int64, 0, 2)
	for _, m := range out[:2] {
		if m.EventType != "broadcast" || m.FromChatID != adminID || m.MessageID != 10 || m.Broadcast != id {
			t.Fatalf("msg = %+v", m)
		}
		chats = append(chats, m.ChatID)
	}
	slices.Sort(chats)
	if !slices.Equal(chats, []int64{42, 43}) {
		t.Fatalf("chats = %v", chats)
	}
	if s := out[2]; s.EventType != "broadcast_summary" || s.ChatID != adminID {
		t.Fatalf("summary = %+v", s)
	}
	if e.count("copyMessage") != previews {
		t.Fatal("direct copyMessage to recipients")
	}
}

func TestBroadcastDoubleTap(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.process(t, tgtest.Callback(adminID, "bc_send"))
	before := len(e.pay.Outbox())

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	if len(e.pay.Outbox()) != before {
		t.Fatal("second tap sent again")
	}
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Рассылка уже отправлена или отменена" {
		t.Fatalf("answer = %q", text)
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
	if out := e.pay.Outbox(); len(out) != 2 || out[0].ChatID != 42 {
		t.Fatalf("outbox = %+v", out)
	}
}

func TestBroadcastPanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	e.rw.FailUsersAfter(1, errors.New("panel down"))

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	if len(e.pay.Outbox()) != 0 {
		t.Fatalf("outbox = %+v", e.pay.Outbox())
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); strings.Contains(text, "очередь") {
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
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, adminID), "await_photo", nil); err != nil {
		t.Fatal(err)
	}
	e.process(t, tgtest.Callback(adminID, "bc_cancel"))

	if st := e.state(t, adminID); st != "await_photo" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "answerCallbackQuery").Params.Get("text"); text != "Рассылка уже отправлена или отменена" {
		t.Fatalf("answer = %q", text)
	}
}

func TestBroadcastMetrics(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(42, now)
	e.rw.AddUser(43, now)
	startBroadcast(t, e)
	bc := testutil.ToFloat64(observability.Broadcasts)

	e.process(t, tgtest.Callback(adminID, "bc_send"))

	wantDelta(t, "broadcasts", bc, testutil.ToFloat64(observability.Broadcasts), 1)
}
