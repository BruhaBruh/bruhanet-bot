package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestTariffsList(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "pay"))
	p := e.last(t, "editMessageText").Params
	if !strings.Contains(p.Get("text"), "Выберите тариф") {
		t.Fatalf("text = %s", p.Get("text"))
	}
	kb := p.Get("reply_markup")
	i1 := strings.Index(kb, `"callback_data":"price:1"`)
	i6 := strings.Index(kb, `"callback_data":"price:6"`)
	i12 := strings.Index(kb, `"callback_data":"price:12"`)
	if i1 < 0 || i6 < i1 || i12 < i6 {
		t.Fatalf("тарифы не по порядку: %s", kb)
	}
	for _, want := range []string{"1 месяц за 150 ₽", "6 месяцев за 800 ₽", `"callback_data":"menu"`} {
		if !strings.Contains(kb, want) {
			t.Fatalf("клавиатура без %s: %s", want, kb)
		}
	}
}

func TestPriceAsksConfirmation(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "price:6"))
	p := e.last(t, "editMessageText").Params
	for _, want := range []string{"6 месяцев", "800 ₽", "Верно?"} {
		if !strings.Contains(p.Get("text"), want) {
			t.Fatalf("text без %q: %s", want, p.Get("text"))
		}
	}
	kb := p.Get("reply_markup")
	if !strings.Contains(kb, `"callback_data":"buy:6"`) || !strings.Contains(kb, `"callback_data":"pay"`) {
		t.Fatalf("клавиатура = %s", kb)
	}
}

func TestPriceUnknownTariffShowsList(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "price:7"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Выберите тариф") {
		t.Fatalf("text = %s", text)
	}
}

func TestBuyCreatesPendingPayment(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if got := e.rw.ExpireAt(userID); !got.Equal(now) {
		t.Fatalf("новый юзер expireAt = %v, want %v", got, now)
	}
	if st := e.state(t, userID); st != "await_photo" {
		t.Fatalf("state = %q", st)
	}
	p := e.pending(t, userID)
	if p.TelegramID != userID || p.Username != "42" || p.Month != 6 || p.Price != 800 || !p.PrevExpireAt.Equal(now) {
		t.Fatalf("payment = %+v", p)
	}
	text := e.last(t, "editMessageText").Params.Get("text")
	for _, want := range []string{"<b>800 ₽</b>", "<code>+7 (800) 555-35-35</code>", "<b>Т-Банк</b>", "- <code>На кофе</code>\n- <code>Перекус</code>", "фото платежа"} {
		if !strings.Contains(text, want) {
			t.Fatalf("реквизиты без %q: %s", want, text)
		}
	}
}

func TestBuyExistingUserKeepsExpire(t *testing.T) {
	e := newEnv(t)
	expire := now.Add(10 * 24 * time.Hour)
	e.rw.AddUser(userID, expire)
	e.process(t, tgtest.Callback(userID, "buy:1"))
	if p := e.pending(t, userID); !p.PrevExpireAt.Equal(expire) {
		t.Fatalf("PrevExpireAt = %v, want %v", p.PrevExpireAt, expire)
	}
}

func TestBuyUnknownTariffDoesNothing(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:7"))
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
	if got := e.rw.ExpireAt(userID); !got.IsZero() {
		t.Fatalf("юзер создан для невалидного тарифа: %v", got)
	}
	if n := len(e.pay.All()); n != 0 {
		t.Fatalf("payments = %d", n)
	}
}

func TestBuyCancelsPaymentInReview(t *testing.T) {
	e := newEnv(t)
	base := now.Add(24 * time.Hour)
	old := inReview(t, e, base) // аванс выдан: в панели base+24h

	e.process(t, tgtest.Callback(userID, "buy:1"))

	if got := e.rw.ExpireAt(userID); !got.Equal(base) {
		t.Fatalf("срок не откатился: %v, want %v", got, base)
	}
	if s := e.status(t, old.ID); s != model.PaymentCancelled {
		t.Fatalf("old status = %q", s)
	}
	p := e.pending(t, userID)
	if e.state(t, userID) != "await_photo" || p.ID == old.ID || p.Month != 1 || !p.PrevExpireAt.Equal(base) || p.AdminMessageID != 0 {
		t.Fatalf("payment = %+v", p)
	}
	edit := e.last(t, "editMessageReplyMarkup").Params
	if edit.Get("chat_id") != "1" || edit.Get("message_id") != "5" {
		t.Fatalf("editMessageReplyMarkup = %v", edit)
	}
	reply := e.last(t, "sendMessage").Params
	if reply.Get("chat_id") != "1" || reply.Get("text") != "Отменён пользователем" || !strings.Contains(reply.Get("reply_parameters"), `"message_id":5`) {
		t.Fatalf("reply = %v", reply)
	}
}

func TestBuyOverPendingWithoutAdvanceKeepsExpire(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	first := e.pending(t, userID)

	e.process(t, tgtest.Callback(userID, "buy:1"))

	if n := len(e.rw.Updates()); n != 0 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if s := e.status(t, first.ID); s != model.PaymentCancelled {
		t.Fatalf("old status = %q", s)
	}
	if p := e.pending(t, userID); p.Month != 1 {
		t.Fatalf("payment = %+v", p)
	}
	if n := e.count("editMessageReplyMarkup") + e.count("sendMessage"); n != 0 {
		t.Fatalf("админ-сообщение тронуто: %d", n)
	}
}

func TestBuyRemnawaveDown(t *testing.T) {
	e := newEnv(t)
	e.rw.FailWith(errors.New("panel is down"))
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
	if n := len(e.pay.All()); n != 0 {
		t.Fatalf("payments = %d", n)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("text = %s", text)
	}
	tgtest.FindLog(t, e.log, "failed to create remnawave user")
	kb := e.last(t, "editMessageText").Params.Get("reply_markup")
	if !strings.Contains(kb, "Назад") || !strings.Contains(kb, `"url":"https://t.me/support"`) {
		t.Fatalf("error keyboard = %s", kb)
	}
}

// Review Focus 5: откат аванса старого платежа упал — ничего не меняется.
func TestBuyCancelFailureKeepsReview(t *testing.T) {
	e := newEnv(t)
	old := inReview(t, e, now.Add(24*time.Hour))
	e.rw.FailUpdateWith(errors.New("panel is down"))

	e.process(t, tgtest.Callback(userID, "buy:1"))

	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
	all := e.pay.All()
	if len(all) != 1 || all[0].ID != old.ID || all[0].Status != model.PaymentPending {
		t.Fatalf("payments = %+v", all)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("text = %s", text)
	}
	if n := e.count("editMessageReplyMarkup"); n != 0 {
		t.Fatalf("админ-сообщение тронуто: %d", n)
	}
	tgtest.FindLog(t, e.log, "failed to create payment")
}
