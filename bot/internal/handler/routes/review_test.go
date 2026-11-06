package routes_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

const adminMessageID = 5

// awaitPhoto создаёт pending-платёж на 6 месяцев со сроком base и стейт await_photo.
func awaitPhoto(t *testing.T, e *env, base time.Time) model.Payment {
	t.Helper()
	p, _, err := e.pay.CreatePayment(t.Context(),
		model.Payment{TelegramID: userID, Month: 6, Price: 800, PrevExpireAt: base}, nil)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, userID), "await_photo", nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return p
}

// inReview — платёж на проверке с выданным авансом, как после фото.
func inReview(t *testing.T, e *env, base time.Time) model.Payment {
	t.Helper()
	e.rw.AddUser(userID, base.Add(24*time.Hour))
	p := awaitPhoto(t, e, base)
	if err := e.pay.SubmitPayment(t.Context(), p.ID, adminMessageID, "file", now); err != nil {
		t.Fatalf("SubmitPayment: %v", err)
	}
	if err := e.pay.GrantAdvance(t.Context(), p.ID, func(model.Payment) {}); err != nil {
		t.Fatalf("GrantAdvance: %v", err)
	}
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, userID), "in_review", nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	return e.pending(t, userID)
}

func adminPress(data string, messageID int) *models.Update {
	u := tgtest.Callback(adminID, data)
	u.CallbackQuery.Message.Message.ID = messageID
	return u
}

func confirmPress(id int64) *models.Update {
	return adminPress(fmt.Sprintf("adm_ok:%d", id), adminMessageID)
}

func declinePress(id int64) *models.Update {
	return adminPress(fmt.Sprintf("adm_no:%d", id), adminMessageID)
}

func (e *env) status(t *testing.T, id int64) model.PaymentStatus {
	t.Helper()
	for _, p := range e.pay.All() {
		if p.ID == id {
			return p.Status
		}
	}
	t.Fatalf("платёж %d не найден", id)
	return ""
}

func (e *env) answer(t *testing.T) string {
	t.Helper()
	return e.last(t, "answerCallbackQuery").Params.Get("text")
}

func TestConfirmExtendsSubscription(t *testing.T) {
	e := newEnv(t)
	p := inReview(t, e, now)

	e.process(t, confirmPress(p.ID))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if s := e.status(t, p.ID); s != model.PaymentConfirmed {
		t.Fatalf("status = %q", s)
	}
	if p := e.last(t, "editMessageReplyMarkup").Params; p.Get("message_id") != "5" {
		t.Fatalf("editMessageReplyMarkup = %v", p)
	}
	if r := e.last(t, "setMessageReaction").Params.Get("reaction"); !strings.Contains(r, "👍") {
		t.Fatalf("reaction = %s", r)
	}
}

func TestDeclineRestoresSubscription(t *testing.T) {
	e := newEnv(t)
	base := now.Add(3 * 24 * time.Hour)
	p := inReview(t, e, base)

	e.process(t, declinePress(p.ID))

	if got := e.rw.ExpireAt(userID); !got.Equal(base) {
		t.Fatalf("expireAt = %v, want %v", got, base)
	}
	if s := e.status(t, p.ID); s != model.PaymentDeclined {
		t.Fatalf("status = %q", s)
	}
	if st := e.state(t, userID); st != "" {
		t.Fatalf("state = %q", st)
	}
	if r := e.last(t, "setMessageReaction").Params.Get("reaction"); !strings.Contains(r, "👎") {
		t.Fatalf("reaction = %s", r)
	}
}

func TestConfirmQueuesNotice(t *testing.T) {
	for _, tc := range []struct {
		name  string
		press func(int64) *models.Update
		key   string
		text  string
	}{
		{"confirm", confirmPress, "payment.confirmed", "<b>Администратор подтвердил ваш платеж</b>\n\nВаша подписка была изменена"},
		{"decline", declinePress, "payment.declined", "<b>Администратор отклонил ваш платеж</b>\n\nВаша подписка была изменена на изначальную"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			p := inReview(t, e, now)

			e.process(t, tc.press(p.ID))

			for _, c := range e.srv.Calls("sendMessage") {
				if c.Params.Get("chat_id") == "42" {
					t.Fatalf("прямое сообщение юзеру: %v", c.Params)
				}
			}
			out := e.pay.Outbox()
			if len(out) != 1 {
				t.Fatalf("outbox = %+v", out)
			}
			m := out[0]
			if m.EventType != "payment_result" || m.ChatID != userID || m.TextKey != tc.key || m.ExpireAt.IsZero() ||
				m.ParseMode != string(models.ParseModeHTML) {
				t.Fatalf("outbox message = %+v", m)
			}
			// текст и клавиатуру рендерит воркер: те же, что раньше клал бот
			text, kb := routes.PaymentNotice(t.Context(), e.c, e.rw, m.ChatID, m.TextKey, m.ExpireAt)
			if text != tc.text || kb == nil {
				t.Fatalf("notice = %q, keyboard = %v", text, kb)
			}
		})
	}
}

func TestDoubleConfirm(t *testing.T) {
	e := newEnv(t)
	p := inReview(t, e, now)
	e.process(t, confirmPress(p.ID))
	e.process(t, confirmPress(p.ID))

	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("UpdateUser calls = %d, want 1", n)
	}
	if text := e.answer(t); text != "Уже обработано" {
		t.Fatalf("второе нажатие: %q", text)
	}
	if n := len(e.pay.Outbox()); n != 1 {
		t.Fatalf("outbox = %d", n)
	}
}

func TestStaleButton(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)

	e.process(t, adminPress("adm_ok:42", 9))

	if text := e.answer(t); text != "Платёж устарел" {
		t.Fatalf("answer = %q", text)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if p := e.last(t, "editMessageReplyMarkup").Params; p.Get("message_id") != "9" {
		t.Fatalf("editMessageReplyMarkup = %v", p)
	}
}

func TestConfirmByNonAdmin(t *testing.T) {
	e := newEnv(t)
	p := inReview(t, e, now)
	u := tgtest.Callback(userID, fmt.Sprintf("adm_ok:%d", p.ID))
	u.CallbackQuery.Message.Message.ID = adminMessageID

	e.process(t, u)

	if text := e.answer(t); text != "Недостаточно прав" {
		t.Fatalf("answer = %q", text)
	}
	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
}

func TestConfirmUpdateFailureKeepsReview(t *testing.T) {
	e := newEnv(t)
	p := inReview(t, e, now)
	e.rw.FailWith(errors.New("panel is down"))

	e.process(t, confirmPress(p.ID))

	if text := e.answer(t); text != "Не удалось обновить пользователя, попробуйте ещё раз" {
		t.Fatalf("answer = %q", text)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	if s := e.status(t, p.ID); s != model.PaymentPending {
		t.Fatalf("status = %q", s)
	}
	if n := e.count("editMessageReplyMarkup"); n != 0 {
		t.Fatalf("кнопки сняты при ошибке: %d", n)
	}

	e.rw.FailWith(nil)
	e.process(t, confirmPress(p.ID))
	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("повтор после ошибки: expireAt = %v, want %v", got, want)
	}
}

func TestConfirmCommitFailRetry(t *testing.T) {
	e := newEnv(t)
	base := now.Add(5 * 24 * time.Hour)
	p := inReview(t, e, base)
	e.pay.FailResolveCommitOnce()

	e.process(t, confirmPress(p.ID))
	if text := e.answer(t); text != "Не удалось обновить пользователя, попробуйте ещё раз" {
		t.Fatalf("answer = %q", text)
	}
	if s := e.status(t, p.ID); s != model.PaymentPending {
		t.Fatalf("status = %q", s)
	}

	e.process(t, confirmPress(p.ID))

	want := base.Add(6 * 30 * 24 * time.Hour)
	if got := e.rw.ExpireAt(userID); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
	ups := e.rw.Updates()
	if len(ups) != 2 || !ups[0].ExpireAt.Equal(want) || !ups[1].ExpireAt.Equal(want) {
		t.Fatalf("updates = %+v", ups)
	}
	if s := e.status(t, p.ID); s != model.PaymentConfirmed {
		t.Fatalf("status = %q", s)
	}
	if n := len(e.pay.Outbox()); n != 1 {
		t.Fatalf("outbox = %d", n)
	}
}

func TestConfirmExpiredSubscriptionCountsFromNow(t *testing.T) {
	e := newEnv(t)
	p := inReview(t, e, now.Add(-90*24*time.Hour))

	e.process(t, confirmPress(p.ID))

	if got, want := e.rw.ExpireAt(userID), now.Add(6*30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
}

func TestDeclineExpiredSubscriptionKeepsBase(t *testing.T) {
	e := newEnv(t)
	base := now.Add(-90 * 24 * time.Hour)
	p := inReview(t, e, base)

	e.process(t, declinePress(p.ID))

	if got := e.rw.ExpireAt(userID); !got.Equal(base) {
		t.Fatalf("expireAt = %v, want %v", got, base)
	}
}
