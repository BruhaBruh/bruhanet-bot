package routes_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func putPayment(t *testing.T, e *env, st fsm.State, p model.Payment) {
	t.Helper()
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, p.TelegramID), st, p); err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func pending(base time.Time) model.Payment {
	return model.Payment{TelegramID: userID, Username: "42", Month: 6, Price: 800, SubscriptionExpireAt: base}
}

func TestPhotoSubmitsForReview(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	putPayment(t, e, "await_photo", pending(now))

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
	if admin.Get("chat_id") != "1" || !strings.Contains(admin.Get("text"), "Покупка подписки BruhaNet") || !strings.Contains(admin.Get("text"), "6 месяцев") {
		t.Fatalf("admin message = %v", admin)
	}
	if kb := admin.Get("reply_markup"); strings.Contains(kb, "adm_ok") {
		t.Fatalf("кнопки до выдачи дня: %s", kb)
	}
	edit := e.last(t, "editMessageReplyMarkup").Params
	if kb := edit.Get("reply_markup"); edit.Get("chat_id") != "1" || edit.Get("message_id") != "1" ||
		!strings.Contains(kb, `"callback_data":"adm_ok:42"`) || !strings.Contains(kb, `"callback_data":"adm_no:42"`) {
		t.Fatalf("admin keyboard edit = %v", edit)
	}
	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	if p := e.payment(t, userID); p.AdminMessageID != 1 {
		t.Fatalf("AdminMessageID = %d", p.AdminMessageID)
	}
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("trial expireAt = %v", got)
	}
	user := sends[1].Params
	if user.Get("chat_id") != "42" || !strings.Contains(user.Get("text"), "выдан доступ на 1 день") {
		t.Fatalf("user message = %v", user)
	}
}

func TestPhotoTrialExtendsFutureExpire(t *testing.T) {
	e := newEnv(t)
	base := now.Add(10 * 24 * time.Hour)
	e.rw.AddUser(userID, base)
	putPayment(t, e, "await_photo", pending(base))
	e.process(t, tgtest.Photo(userID))
	if got := e.rw.ExpireAt(userID); !got.Equal(base.Add(24 * time.Hour)) {
		t.Fatalf("trial expireAt = %v", got)
	}
}

func TestPhotoTrialFailureStillInReview(t *testing.T) {
	e := newEnv(t) // юзера в панели нет — UpdateUser упадёт
	putPayment(t, e, "await_photo", pending(now))
	e.process(t, tgtest.Photo(userID))
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	text := e.last(t, "sendMessage").Params.Get("text")
	if !strings.Contains(text, "на проверку") || strings.Contains(text, "1 день") {
		t.Fatalf("user message = %s", text)
	}
	if l := tgtest.FindLog(t, e.log, "failed to grant trial day"); l["level"] != "ERROR" {
		t.Fatalf("line = %v", l)
	}
}

func TestPhotoInReviewForwardsSilently(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(24*time.Hour))
	p := pending(now)
	p.AdminMessageID = 77
	putPayment(t, e, "in_review", p)

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

// raceStore имитирует второе фото того же альбома, которое успело перевести стейт первым.
type raceStore struct {
	fsm.Storage
}

func (s raceStore) CompareAndSet(ctx context.Context, id int64, from fsm.State, r fsm.Record) error {
	if from == "await_photo" {
		cur, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		cur.State = "in_review"
		if err := s.Set(ctx, id, cur); err != nil {
			return err
		}
	}
	return s.Storage.CompareAndSet(ctx, id, from, r)
}

func TestPhotoRaceLoserClosesItsAdminMessage(t *testing.T) {
	e := newEnvWithStore(t, raceStore{Storage: fsm.NewInMemory()})
	e.rw.AddUser(userID, now)
	putPayment(t, e, "await_photo", pending(now))

	e.process(t, tgtest.Photo(userID))

	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("проигравшее фото выдало день: %d", n)
	}
	reply := e.last(t, "sendMessage").Params
	if reply.Get("chat_id") != "1" || reply.Get("text") != "Повторное фото этого платежа" {
		t.Fatalf("reply = %v", reply)
	}
	edits := e.srv.Calls("editMessageReplyMarkup")
	if len(edits) != 1 || strings.Contains(edits[0].Params.Get("reply_markup"), "adm_ok") {
		t.Fatalf("проигравший навесил кнопки: %v", edits)
	}
}

func TestPhotoTrialFromNowWhenExpired(t *testing.T) {
	e := newEnv(t)
	base := now.Add(-48 * time.Hour)
	e.rw.AddUser(userID, base)
	putPayment(t, e, "await_photo", pending(base))
	e.process(t, tgtest.Photo(userID))
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("trial expireAt = %v", got)
	}
}

// failCAS возвращает обычную ошибку (не mismatch) на CompareAndSet с from→to.
type failCAS struct {
	fsm.Storage
	from, to fsm.State
}

func (s failCAS) CompareAndSet(ctx context.Context, id int64, from fsm.State, r fsm.Record) error {
	if from == s.from && r.State == s.to {
		return errors.New("storage down")
	}
	return s.Storage.CompareAndSet(ctx, id, from, r)
}

func TestPhotoTransitionErrorIsNotRace(t *testing.T) {
	e := newEnvWithStore(t, failCAS{Storage: fsm.NewInMemory(), from: "await_photo", to: "in_review"})
	e.rw.AddUser(userID, now)
	putPayment(t, e, "await_photo", pending(now))

	e.process(t, tgtest.Photo(userID))

	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("user text = %s", text)
	}
	var found bool
	for _, c := range e.srv.Calls("sendMessage") {
		if c.Params.Get("chat_id") == "1" && c.Params.Get("text") == "Ошибка сохранения платежа" {
			found = true
		}
	}
	if !found {
		t.Fatal("админ не получил пометку об ошибке")
	}
	if l := tgtest.FindLog(t, e.log, "failed to move payment to review"); l["level"] != "ERROR" {
		t.Fatalf("line = %v", l)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("день выдан: %d", n)
	}
}
