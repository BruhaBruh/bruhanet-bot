package routes_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestPhotoSubmitsForReview(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	p := awaitPhoto(t, e, now)

	e.process(t, tgtest.Photo(userID))

	cp := e.last(t, "copyMessage").Params
	if cp.Get("chat_id") != "1" || cp.Get("from_chat_id") != "42" || cp.Get("message_id") != "10" {
		t.Fatalf("copyMessage = %v", cp)
	}
	sends := e.srv.Calls("sendMessage")
	if len(sends) != 2 {
		t.Fatalf("sendMessage calls = %d", len(sends))
	}
	admin := sends[0].Params
	if text := admin.Get("text"); admin.Get("chat_id") != "1" || !strings.Contains(text, "Покупка подписки BruhaNet") ||
		!strings.Contains(text, "6 месяцев") || !strings.HasSuffix(text, "\n\n🆕 первая оплата") {
		t.Fatalf("admin message = %v", admin)
	}
	if kb := admin.Get("reply_markup"); strings.Contains(kb, "adm_ok") {
		t.Fatalf("кнопки до выдачи дня: %s", kb)
	}
	edit := e.last(t, "editMessageReplyMarkup").Params
	if kb := edit.Get("reply_markup"); edit.Get("chat_id") != "1" || edit.Get("message_id") != "1" ||
		!strings.Contains(kb, fmt.Sprintf(`"callback_data":"adm_ok:%d"`, p.ID)) ||
		!strings.Contains(kb, fmt.Sprintf(`"callback_data":"adm_no:%d"`, p.ID)) {
		t.Fatalf("admin keyboard edit = %v", edit)
	}
	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	got := e.pending(t, userID)
	if got.AdminMessageID != 1 || got.SubmittedAt == nil || !got.AdvanceGranted {
		t.Fatalf("payment = %+v", got)
	}
	if _, err := fsm.Data[map[string]any](t.Context(), fsm.For(e.fsm, userID)); err == nil {
		t.Fatal("в FSM остались данные платежа")
	}
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("advance expireAt = %v", got)
	}
	user := sends[1].Params
	if user.Get("chat_id") != "42" || !strings.Contains(user.Get("text"), "выдан доступ на 1 день") {
		t.Fatalf("user message = %v", user)
	}
}

func TestPhotoAdvanceExtendsFutureExpire(t *testing.T) {
	e := newEnv(t)
	base := now.Add(10 * 24 * time.Hour)
	e.rw.AddUser(userID, base)
	awaitPhoto(t, e, base)
	e.process(t, tgtest.Photo(userID))
	if got := e.rw.ExpireAt(userID); !got.Equal(base.Add(24 * time.Hour)) {
		t.Fatalf("advance expireAt = %v", got)
	}
}

func TestPhotoAdvanceFromNowWhenExpired(t *testing.T) {
	e := newEnv(t)
	base := now.Add(-48 * time.Hour)
	e.rw.AddUser(userID, base)
	awaitPhoto(t, e, base)
	e.process(t, tgtest.Photo(userID))
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("advance expireAt = %v", got)
	}
}

func TestPhotoAdvanceFailureStillInReview(t *testing.T) {
	e := newEnv(t) // юзера в панели нет — UpdateUser упадёт
	awaitPhoto(t, e, now)
	e.process(t, tgtest.Photo(userID))
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	if p := e.pending(t, userID); p.AdvanceGranted || p.SubmittedAt == nil {
		t.Fatalf("payment = %+v", p)
	}
	text := e.last(t, "sendMessage").Params.Get("text")
	if !strings.Contains(text, "на проверку") || strings.Contains(text, "1 день") {
		t.Fatalf("user message = %s", text)
	}
	if l := tgtest.FindLog(t, e.log, "failed to grant advance day"); l["level"] != "ERROR" {
		t.Fatalf("line = %v", l)
	}
}

func TestPhotoInReviewForwardsSilently(t *testing.T) {
	e := newEnv(t)
	p := awaitPhoto(t, e, now)
	if err := e.pay.SubmitPayment(t.Context(), p.ID, 77, now); err != nil {
		t.Fatal(err)
	}
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, userID), "in_review", nil); err != nil {
		t.Fatal(err)
	}

	e.process(t, tgtest.Photo(userID))

	cp := e.last(t, "copyMessage").Params
	if cp.Get("chat_id") != "1" || !strings.Contains(cp.Get("reply_parameters"), `"message_id":77`) {
		t.Fatalf("copyMessage = %v", cp)
	}
	if n := e.count("sendMessage"); n != 0 {
		t.Fatalf("sendMessage calls = %d", n)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
}

func TestPhotoWithoutPaymentShowsMenu(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Photo(userID))
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Здравствуйте") {
		t.Fatalf("text = %s", text)
	}
}

func TestPhotoAwaitStateWithoutPendingShowsMenu(t *testing.T) {
	e := newEnv(t)
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, userID), "await_photo", nil); err != nil {
		t.Fatal(err)
	}
	e.process(t, tgtest.Photo(userID))
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "Здравствуйте") {
		t.Fatalf("text = %s", text)
	}
	if n := e.count("copyMessage"); n != 0 {
		t.Fatalf("copyMessage calls = %d", n)
	}
}

func TestAlbumPhotos(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	p := awaitPhoto(t, e, now)

	e.process(t, tgtest.Photo(userID))
	e.process(t, tgtest.Photo(userID))

	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("аванс выдан %d раз", n)
	}
	copies := e.srv.Calls("copyMessage")
	if len(copies) != 2 || copies[0].Params.Get("reply_parameters") != "" ||
		!strings.Contains(copies[1].Params.Get("reply_parameters"), `"message_id":1`) {
		t.Fatalf("copies = %+v", copies)
	}
	var reviews int
	for _, c := range e.srv.Calls("sendMessage") {
		if strings.Contains(c.Params.Get("text"), "Покупка подписки") {
			reviews++
		}
	}
	if reviews != 1 {
		t.Fatalf("сообщений проверки = %d", reviews)
	}
	if edit := e.last(t, "editMessageReplyMarkup").Params.Get("reply_markup"); !strings.Contains(edit, fmt.Sprintf("adm_ok:%d", p.ID)) {
		t.Fatalf("кнопки = %s", edit)
	}
}

func TestSubmitRace(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	awaitPhoto(t, e, now)
	e.pay.FailSubmitNotPendingOnce()

	e.process(t, tgtest.Photo(userID))

	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("проигравшее фото выдало день: %d", n)
	}
	reply := e.last(t, "sendMessage").Params
	if reply.Get("chat_id") != "1" || reply.Get("text") != "Повторное фото этого платежа" ||
		!strings.Contains(reply.Get("reply_parameters"), `"message_id":1`) {
		t.Fatalf("reply = %v", reply)
	}
	edits := e.srv.Calls("editMessageReplyMarkup")
	if len(edits) != 1 || strings.Contains(edits[0].Params.Get("reply_markup"), "adm_ok") {
		t.Fatalf("проигравший навесил кнопки: %v", edits)
	}
}
