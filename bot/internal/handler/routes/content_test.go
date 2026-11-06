package routes_test

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
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
	text, _ := routes.PaymentNotice(t.Context(), e.c, e.rw, out[0].ChatID, out[0].TextKey, out[0].ExpireAt, 0)
	if text != want || out[0].ChatID != userID {
		t.Fatalf("got %q, want %q", text, want)
	}
}

// keyboardButtons — разметка из reply_markup вызова Telegram.
func keyboardButtons(t *testing.T, raw string) []models.InlineKeyboardButton {
	t.Helper()
	var kb models.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(raw), &kb); err != nil {
		t.Fatalf("reply_markup %q: %v", raw, err)
	}
	return slices.Concat(kb.InlineKeyboard...)
}

// Реестр говорит правду: каждая кнопка на экране объявлена в Def.Buttons сообщения, действие совпадает с Def.Route.
func TestDeclaredButtons(t *testing.T) {
	e := newEnv(t)
	byLabel := map[string]texts.Def{}
	for _, d := range texts.Registry() {
		if d.Kind == texts.KindButton {
			byLabel[d.Default] = d
		}
	}
	check := func(key, markup string) {
		t.Helper()
		def, ok := texts.Lookup(key)
		if !ok {
			t.Fatalf("no %s", key)
		}
		for _, b := range keyboardButtons(t, markup) {
			btn, ok := byLabel[b.Text]
			if !ok && strings.Contains(b.Text, " за ") {
				btn, ok = texts.Lookup("button.tariff")
			}
			if !ok || !slices.Contains(def.Buttons, btn.Key) {
				t.Errorf("%s: кнопка %q не объявлена в %v", key, b.Text, def.Buttons)
				continue
			}
			route, _, _ := strings.Cut(b.CallbackData, ":")
			switch {
			case b.URL != "":
				route = "url:"
			case b.CopyText != nil:
				route = "copy:"
			}
			if !strings.Contains(btn.Route, route) {
				t.Errorf("%s: кнопка %q ведёт на %q, в реестре %q", key, b.Text, route, btn.Route)
			}
		}
	}
	edited := func() string { return e.last(t, "editMessageText").Params.Get("reply_markup") }
	sent := func() string { return e.last(t, "sendMessage").Params.Get("reply_markup") }

	e.process(t, tgtest.Message(userID, "/start"))
	check("menu.greeting", sent()) // нет в панели
	e.process(t, tgtest.Callback(userID, "info"))
	check("menu.info_not_bought", edited())
	e.rw.AddUser(userID, now)
	e.process(t, tgtest.Message(userID, "/start"))
	check("menu.greeting", sent())
	e.process(t, tgtest.Callback(userID, "info"))
	check("menu.info", edited())
	e.process(t, tgtest.Callback(userID, "pay"))
	check("payment.tariffs", edited())
	e.process(t, tgtest.Callback(userID, "price:6"))
	check("payment.confirm", edited())
	e.process(t, tgtest.Callback(userID, "have_promo:6"))
	check("promo.enter_payment", edited())
	e.process(t, tgtest.Message(userID, "NOPE"))
	check("promo.not_found", sent()) // ответ на сообщение — новое сообщение, а не правка
	e.process(t, tgtest.Message(userID, "/start"))
	e.process(t, tgtest.Callback(userID, "promo"))
	check("promo.enter_menu", edited())
	e.process(t, tgtest.Message(userID, "/start"))
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))
	check("payment.photo_sent_advance", sent())

	c := content.New(e.src, settings.Defaults(map[int]int{1: 150}, "+7", "Банк", "https://t.me/s"), 0, nil)
	_, kb := routes.ReminderMessage(t.Context(), c, "day", now, "https://sub")
	raw, err := json.Marshal(kb)
	if err != nil {
		t.Fatal(err)
	}
	check("reminder.day", string(raw))
}

// Без сохранённых стилей вид как был; сохранённый применяется всюду, где кнопка из реестра.
func TestButtonStyle(t *testing.T) {
	for _, st := range []string{string(keyboard.StyleDefault), string(keyboard.StylePrimary), string(keyboard.StyleSuccess), string(keyboard.StyleDanger)} {
		if !slices.Contains(texts.Styles, st) {
			t.Fatalf("keyboard style %q is not in texts.Styles", st)
		}
	}
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	styles := func() map[string]string {
		out := map[string]string{}
		for _, b := range keyboardButtons(t, e.last(t, "sendMessage").Params.Get("reply_markup")) {
			out[b.Text] = b.Style
		}
		return out
	}
	e.process(t, tgtest.Message(userID, "/start"))
	want := map[string]string{"Информация": "primary", "Оплатить": "", "Подписка": "success", "Поддержка": "danger", "Промокод": ""}
	if got := styles(); !maps.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Def.Style — вид главного меню
	for label, st := range want {
		for _, d := range texts.Registry() {
			if d.Kind == texts.KindButton && d.Default == label && d.Style != st {
				t.Errorf("%s: Def.Style %q, в меню %q", d.Key, d.Style, st)
			}
		}
	}

	e.src.SetStyles(map[string]string{"button.pay": "danger", "button.info": ""})
	e.process(t, tgtest.Message(userID, "/start"))
	got := styles()
	if got["Оплатить"] != "danger" || got["Информация"] != "" || got["Подписка"] != "success" {
		t.Fatalf("got %v", got)
	}
	// подпись тарифа — та же кнопка реестра
	e.src.SetStyles(map[string]string{"button.tariff": "primary", "button.yes": "danger"})
	e.process(t, tgtest.Callback(userID, "pay"))
	if b := keyboardButtons(t, e.last(t, "editMessageText").Params.Get("reply_markup"))[0]; b.Style != "primary" {
		t.Fatalf("tariff = %+v", b)
	}
	e.process(t, tgtest.Callback(userID, "price:6"))
	if b := keyboardButtons(t, e.last(t, "editMessageText").Params.Get("reply_markup"))[0]; b.Text != "Да" || b.Style != "danger" {
		t.Fatalf("yes = %+v", b)
	}
}
