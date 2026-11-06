package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

// Правка текста и настроек из админки (фейковый Source) доходит до юзера.
func TestTextOverrideReachesUser(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	e.src.SetTexts(map[string]string{"menu.greeting": "Привет, <i>{{.name}}</i>!", "button.pay": "Платить"})
	e.src.SetSettings(map[string][]byte{"supportUrl": []byte(`"https://t.me/help"`)})

	e.process(t, tgtest.Message(userID, "/start"))

	p := e.last(t, "sendMessage").Params
	if p.Get("text") != "Привет, <i>Test</i>!" || p.Get("parse_mode") != "HTML" {
		t.Fatalf("params = %v", p)
	}
	if kb := p.Get("reply_markup"); !strings.Contains(kb, `"text":"Платить"`) || !strings.Contains(kb, `"url":"https://t.me/help"`) {
		t.Fatalf("reply_markup = %s", kb)
	}
}

// Тарифы и реквизиты — из настроек; тариф, которого нет в настройках, по подделанному callback не продаётся.
func TestTariffsFromSettings(t *testing.T) {
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{
		"prices":   []byte(`{"3":300}`),
		"comments": []byte(`["a<b"]`),
	})

	e.process(t, tgtest.Callback(userID, "pay"))
	kb := e.last(t, "editMessageText").Params.Get("reply_markup")
	if !strings.Contains(kb, "3 месяца за 300 ₽") || !strings.Contains(kb, `"callback_data":"price:3"`) || strings.Contains(kb, "price:1") {
		t.Fatalf("tariffs = %s", kb)
	}

	e.process(t, tgtest.Callback(userID, "price:1")) // тариф из env, удалён в админке
	if kb := e.last(t, "editMessageText").Params.Get("reply_markup"); strings.Contains(kb, "buy:1") {
		t.Fatalf("удалённый тариф предложен: %s", kb)
	}

	e.process(t, tgtest.Callback(userID, "price:3"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "<b>3 месяца</b> за <b>300 ₽</b>") {
		t.Fatalf("confirm = %s", text)
	}

	e.process(t, tgtest.Callback(userID, "buy:1")) // подделанная покупка удалённого тарифа
	if ps := e.pay.All(); len(ps) != 0 {
		t.Fatalf("платёж по удалённому тарифу: %+v", ps)
	}

	e.process(t, tgtest.Callback(userID, "buy:3"))
	text := e.last(t, "editMessageText").Params.Get("text")
	if !strings.Contains(text, "с одним из комментариев:\n- <code>a&lt;b</code>\nи отправьте") {
		t.Fatalf("requisites = %s", text)
	}
	if p := e.pending(t, userID); p.Month != 3 || p.Price != 300 {
		t.Fatalf("payment = %+v", p)
	}
}

// Review Focus 2: username и имя с HTML-символами в сообщении проверки админу экранированы.
func TestReviewMessageEscapesUsername(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	_, _, err := e.pay.CreatePayment(t.Context(),
		model.Payment{TelegramID: userID, Username: "a<b>", Month: 6, Price: 800, PrevExpireAt: now}, nil)
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if err := fsm.Put(t.Context(), fsm.For(e.fsm, userID), "await_photo", nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	u := tgtest.Photo(userID)
	u.Message.From.FirstName = "Tom & <i>"

	e.process(t, u)

	admin := e.srv.Calls("sendMessage")[0].Params
	want := "Пользователь <b>Tom &amp; &lt;i&gt;</b> @a&lt;b&gt; (<b>42</b>) купил подписку на <b>6 месяцев</b>"
	if admin.Get("chat_id") != "1" || admin.Get("parse_mode") != "HTML" || !strings.Contains(admin.Get("text"), want) {
		t.Fatalf("admin message = %v", admin)
	}
}

const nameBody = "Привет, {{.name}} ({{.telegramId}})"

// {{.name}} в экране по callback: данные автора апдейта, значение экранировано.
func TestGlobalVarInCallbackScreen(t *testing.T) {
	e := newEnv(t)
	e.src.SetTexts(map[string]string{"payment.tariffs": nameBody})
	e.process(t, tgtest.Callback(userID, "pay"))
	if got, want := e.last(t, "editMessageText").Params.Get("text"), "Привет, Test (42)"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// {{.name}} в уведомлении через outbox: адресат — юзер платежа, а не нажавший кнопку админ.
func TestGlobalVarInOutboxMessage(t *testing.T) {
	e := newEnv(t)
	e.src.SetTexts(map[string]string{"payment.confirmed": nameBody + " до {{.expireAt}}"})
	e.src.SetContact(userID, model.UserContact{FirstName: "<Юзер>"})
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(e.pending(t, userID).ID))
	out := e.pay.Outbox()
	if len(out) != 1 {
		t.Fatalf("outbox = %+v", out)
	}
	// срок — тот, что только что записан в панель, а не прежний из users
	want := "Привет, &lt;Юзер&gt; (42) до " + "9 июля 2026 г. в 15:00, MSK"
	if out[0].Text != want || out[0].ChatID != userID {
		t.Fatalf("got %q, want %q", out[0].Text, want)
	}
}
