package routes_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func ptr[T any](v T) *T { return &v }

// enterPaymentPromo: «Верно?» → «Есть промокод» → код сообщением; возвращает текст ответа.
func enterPaymentPromo(t *testing.T, e *env, code string) string {
	t.Helper()
	e.process(t, tgtest.Callback(userID, "have_promo:1"))
	e.process(t, tgtest.Message(userID, code))
	return e.last(t, "sendMessage").Params.Get("text")
}

func TestPriceOffersPromo(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "price:6"))
	if kb := e.last(t, "editMessageText").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"have_promo:6"`) {
		t.Fatalf("клавиатура = %s", kb)
	}
}

func TestHavePromoAsksCode(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "have_promo:6"))
	if st := e.state(t, userID); st != "await_promo" {
		t.Fatalf("state = %q", st)
	}
	p := e.last(t, "editMessageText").Params
	if !strings.Contains(p.Get("text"), "Отправьте промокод сообщением") || !strings.Contains(p.Get("reply_markup"), `"callback_data":"price:6"`) {
		t.Fatalf("экран = %v", p)
	}
	// подделанный тариф — к списку тарифов, стейт не меняется
	e2 := newEnv(t)
	e2.process(t, tgtest.Callback(userID, "have_promo:7"))
	if st := e2.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
}

// Полный путь скидки 25%: 150 → 112 ₽, код с пробелами и в нижнем регистре (Review Focus 5).
func TestPromoDiscountPath(t *testing.T) {
	e := newEnv(t)
	pr := e.pay.AddPromo(model.Promo{Code: "SUMMER25", Kind: model.PromoPercent, Value: 25})

	text := enterPaymentPromo(t, e, "  summer25 ")
	for _, want := range []string{"<s>150 ₽</s> <b>112 ₽</b>", "<code>SUMMER25</code>", "1 месяц"} {
		if !strings.Contains(text, want) {
			t.Fatalf("экран без %q: %s", want, text)
		}
	}
	kb := e.last(t, "sendMessage").Params.Get("reply_markup")
	for _, want := range []string{`"callback_data":"buyp:1:1"`, `"callback_data":"pay"`, `"callback_data":"price:1"`, "Убрать промокод"} {
		if !strings.Contains(kb, want) {
			t.Fatalf("клавиатура без %s: %s", want, kb)
		}
	}
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("после ввода кода state = %q", st)
	}

	e.process(t, tgtest.Callback(userID, "buyp:1:1"))
	p := e.pending(t, userID)
	if p.Price != 112 || p.BasePrice == nil || *p.BasePrice != 150 || p.PromoID == nil || *p.PromoID != pr.ID || p.Month != 1 {
		t.Fatalf("payment = %+v", p)
	}
	if st := e.state(t, userID); st != "await_photo" {
		t.Fatalf("state = %q", st)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Переведите <b>112 ₽</b>") {
		t.Fatalf("реквизиты = %s", text)
	}
	if n := len(e.pay.Activations()); n != 0 {
		t.Fatalf("активация до подтверждения: %d", n)
	}
}

func TestPromoPhotoShowsPromoToAdmin(t *testing.T) {
	e := newEnv(t)
	e.pay.AddPromo(model.Promo{Code: "MINUS100", Kind: model.PromoRub, Value: 100})
	enterPaymentPromo(t, e, "minus100")
	e.process(t, tgtest.Callback(userID, "buyp:1:1"))
	e.process(t, tgtest.Photo(userID))
	var found bool
	for _, c := range e.srv.Calls("sendMessage") {
		if c.Params.Get("chat_id") == "1" && strings.Contains(c.Params.Get("text"), "🎟 промокод MINUS100 (−100 ₽)") {
			found = true
		}
	}
	if !found {
		t.Fatal("админу не показан промокод")
	}
}

// Цена 0: платёж сразу confirmed, срок продлён, уведомление в outbox, админу — инфо без кнопок.
// Повторное «Да» не продлевает второй раз (Review Focus 2).
func TestPromoFreeConfirmsImmediately(t *testing.T) {
	e := newEnv(t)
	expire := now.Add(10 * 24 * time.Hour)
	e.rw.AddUser(userID, expire)
	e.pay.AddPromo(model.Promo{Code: "FREE", Kind: model.PromoPercent, Value: 100})

	enterPaymentPromo(t, e, "free")
	e.process(t, tgtest.Callback(userID, "buyp:1:1"))

	all := e.pay.All()
	if len(all) != 1 || all[0].Status != model.PaymentConfirmed || all[0].Price != 0 || *all[0].BasePrice != 150 {
		t.Fatalf("payments = %+v", all)
	}
	want := expire.Add(30 * 24 * time.Hour)
	if got := e.rw.ExpireAt(userID); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
	out := e.pay.Outbox()
	if len(out) != 1 || out[0].EventType != "payment_result" || out[0].ChatID != userID ||
		!strings.Contains(out[0].Text, "оплачена промокодом") || !strings.Contains(out[0].Text, "1 месяц") {
		t.Fatalf("outbox = %+v", out)
	}
	if n := len(e.pay.Activations()); n != 1 {
		t.Fatalf("activations = %d", n)
	}
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
	admin := adminMessages(e)
	if len(admin) != 1 || !strings.Contains(admin[0].Params.Get("text"), "FREE") || admin[0].Params.Get("reply_markup") != "" {
		t.Fatalf("админу = %+v", admin)
	}

	e.process(t, tgtest.Callback(userID, "buyp:1:1"))
	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("UpdateUser calls = %d", n)
	}
	if n := len(e.pay.All()); n != 1 {
		t.Fatalf("payments = %d", n)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "уже использовали") {
		t.Fatalf("повтор = %s", text)
	}
}

// Двойное нажатие «Да» одновременно: одно продление.
func TestPromoFreeConcurrentPresses(t *testing.T) {
	for range 20 {
		e := newEnv(t)
		e.rw.AddUser(userID, now)
		e.pay.AddPromo(model.Promo{Code: "FREE", Kind: model.PromoRub, Value: 1000})
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() { e.process(t, tgtest.Callback(userID, "buyp:1:1")) })
		}
		wg.Wait()
		if n := len(e.rw.Updates()); n != 1 {
			t.Fatalf("UpdateUser calls = %d", n)
		}
		var confirmed int
		for _, p := range e.pay.All() {
			if p.Status == model.PaymentConfirmed {
				confirmed++
			}
		}
		if confirmed != 1 {
			t.Fatalf("confirmed = %d: %+v", confirmed, e.pay.All())
		}
	}
}

func TestPromoFreePanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	e.pay.AddPromo(model.Promo{Code: "FREE", Kind: model.PromoPercent, Value: 100})
	e.rw.FailUpdateWith(errors.New("panel is down"))
	e.process(t, tgtest.Callback(userID, "buyp:1:1"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("text = %s", text)
	}
	if n := len(e.pay.Activations()); n != 0 {
		t.Fatalf("activations = %d", n)
	}
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
	// панель ожила — повтор проходит
	e.rw.FailUpdateWith(nil)
	e.process(t, tgtest.Callback(userID, "buyp:1:1"))
	if n := len(e.pay.Activations()); n != 1 {
		t.Fatalf("activations = %d", n)
	}
}

func adminMessages(e *env) []tgtest.Call {
	var out []tgtest.Call
	for _, c := range e.srv.Calls("sendMessage") {
		if c.Params.Get("chat_id") == "1" {
			out = append(out, c)
		}
	}
	return out
}

// confirmedWith — у юзера подтверждённая оплата (с промокодом promoID, если не nil).
func confirmedWith(t *testing.T, e *env, promoID *int64) {
	t.Helper()
	p, _, err := e.pay.CreatePayment(t.Context(), model.Payment{TelegramID: userID, Month: 1, Price: 150, PromoID: promoID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pay.ResolvePayment(t.Context(), p.ID, model.PaymentConfirmed, now,
		func(model.Payment) (model.OutboxMessage, error) { return model.OutboxMessage{}, nil }); err != nil {
		t.Fatal(err)
	}
}

func TestPromoRejectReasons(t *testing.T) {
	for _, tc := range []struct {
		name  string
		promo model.Promo
		setup func(t *testing.T, e *env, id int64)
		menu  bool
		want  string
	}{
		{name: "not found", want: "Такого промокода нет"},
		{name: "disabled", promo: model.Promo{Disabled: true}, want: "больше не действует"},
		{name: "expired", promo: model.Promo{ExpiresAt: ptr(now)}, want: "Срок действия этого промокода истёк"},
		{name: "exhausted", promo: model.Promo{MaxActivations: ptr(0)}, want: "Активации этого промокода закончились"},
		{name: "not yours", promo: model.Promo{TelegramID: ptr[int64](7)}, want: "предназначен другому пользователю"},
		{
			name: "only new", promo: model.Promo{OnlyNew: true}, want: "только для новых",
			setup: func(t *testing.T, e *env, _ int64) { confirmedWith(t, e, nil) },
		},
		{
			name: "already used", want: "уже использовали",
			setup: func(t *testing.T, e *env, id int64) { confirmedWith(t, e, &id) },
		},
		{name: "days at payment", promo: model.Promo{Kind: model.PromoDays, Value: 7}, want: "через кнопку «Промокод» в меню"},
		{name: "discount from menu", menu: true, want: "введите её при оплате тарифа"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			code := "NOPE"
			if tc.name != "not found" {
				pr := tc.promo
				pr.Code = "CODE1"
				if pr.Kind == "" {
					pr.Kind, pr.Value = model.PromoPercent, 25
				}
				pr = e.pay.AddPromo(pr)
				code = pr.Code
				if tc.setup != nil {
					tc.setup(t, e, pr.ID)
				}
			}
			before := len(e.pay.All())
			wantState := fsm.State("await_promo")
			if tc.menu {
				wantState = "await_promo_days"
				e.process(t, tgtest.Callback(userID, "promo"))
				e.process(t, tgtest.Message(userID, code))
			} else {
				enterPaymentPromo(t, e, code)
			}
			if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, tc.want) {
				t.Fatalf("text = %s", text)
			}
			if st := e.state(t, userID); st != wantState {
				t.Fatalf("state = %q, ждём код снова", st)
			}
			if n := len(e.pay.All()); n != before {
				t.Fatalf("payments = %d", n)
			}
		})
	}
}

// Подделанный buyp (Review Focus 3): платёж не создаётся.
func TestBuypForged(t *testing.T) {
	for _, tc := range []struct {
		name, data string
	}{
		{"чужой код", "buyp:1:1"},
		{"нет тарифа", "buyp:7:2"},
		{"код на дни", "buyp:1:3"},
		{"нет кода", "buyp:1:99"},
		{"мусор", "buyp:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.pay.AddPromo(model.Promo{Code: "OTHER", Kind: model.PromoPercent, Value: 100, TelegramID: ptr[int64](7)})
			e.pay.AddPromo(model.Promo{Code: "FREE", Kind: model.PromoPercent, Value: 100})
			e.pay.AddPromo(model.Promo{Code: "DAYS", Kind: model.PromoDays, Value: 30})
			e.process(t, tgtest.Callback(userID, tc.data))
			if n := len(e.pay.All()); n != 0 {
				t.Fatalf("payments = %+v", e.pay.All())
			}
			if n := len(e.rw.Updates()); n != 0 {
				t.Fatalf("updates = %d", n)
			}
			if got := e.rw.ExpireAt(userID); !got.IsZero() {
				t.Fatalf("юзер создан: %v", got)
			}
		})
	}
}

func TestMenuHasPromoButton(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Message(userID, "/start"))
	if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"promo"`) {
		t.Fatalf("без юзера: %s", kb)
	}
	e.rw.AddUser(userID, now)
	e.process(t, tgtest.Message(userID, "/start"))
	if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"promo"`) {
		t.Fatalf("с юзером: %s", kb)
	}
}

func TestDaysPromoFromMenu(t *testing.T) {
	e := newEnv(t)
	expire := now.Add(5 * 24 * time.Hour)
	e.rw.AddUser(userID, expire)
	e.pay.AddPromo(model.Promo{Code: "WEEK", Kind: model.PromoDays, Value: 7})

	e.process(t, tgtest.Callback(userID, "promo"))
	if st := e.state(t, userID); st != "await_promo_days" {
		t.Fatalf("state = %q", st)
	}
	p := e.last(t, "editMessageText").Params
	if !strings.Contains(p.Get("text"), "Отправьте промокод") || !strings.Contains(p.Get("reply_markup"), `"callback_data":"menu"`) {
		t.Fatalf("экран = %v", p)
	}
	e.process(t, tgtest.Message(userID, " week "))
	want := expire.Add(7 * 24 * time.Hour)
	if got := e.rw.ExpireAt(userID); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
	text := e.last(t, "sendMessage").Params.Get("text")
	if !strings.Contains(text, "Добавлено дней: <b>7</b>") || !strings.Contains(text, "22 января 2026") {
		t.Fatalf("text = %s", text)
	}
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}

	e.process(t, tgtest.Callback(userID, "promo"))
	e.process(t, tgtest.Message(userID, "WEEK"))
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "уже использовали") {
		t.Fatalf("повтор = %s", text)
	}
	if n := len(e.rw.Updates()); n != 1 {
		t.Fatalf("updates = %d", n)
	}
}

func TestDaysPromoNewUser(t *testing.T) {
	e := newEnv(t)
	e.pay.AddPromo(model.Promo{Code: "WEEK", Kind: model.PromoDays, Value: 7})
	e.process(t, tgtest.Callback(userID, "promo"))
	e.process(t, tgtest.Message(userID, "week"))
	if got, want := e.rw.ExpireAt(userID), now.Add(7*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expireAt = %v, want %v", got, want)
	}
}

// Ошибка панели (Review Focus 4): активации нет, код можно ввести снова.
func TestDaysPromoPanelError(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	e.pay.AddPromo(model.Promo{Code: "WEEK", Kind: model.PromoDays, Value: 7})
	e.rw.FailUpdateWith(errors.New("panel is down"))
	e.process(t, tgtest.Callback(userID, "promo"))
	e.process(t, tgtest.Message(userID, "week"))
	if text := e.last(t, "sendMessage").Params.Get("text"); !strings.Contains(text, "произошла ошибка") {
		t.Fatalf("text = %s", text)
	}
	if n := len(e.pay.Activations()); n != 0 {
		t.Fatalf("activations = %d", n)
	}
	e.rw.FailUpdateWith(nil)
	e.process(t, tgtest.Message(userID, "week"))
	if n := len(e.pay.Activations()); n != 1 {
		t.Fatalf("activations = %d", n)
	}
}

// /start из ожидания кода — меню; стейт оплаты восстанавливается по pending-платежу.
func TestStartLeavesPromoState(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "have_promo:1"))
	e.process(t, tgtest.Message(userID, "/start"))
	if st := e.state(t, userID); st != fsm.StateNone {
		t.Fatalf("state = %q", st)
	}
	if kb := e.last(t, "sendMessage").Params.Get("reply_markup"); !strings.Contains(kb, `"callback_data":"pay"`) {
		t.Fatalf("клавиатура = %s", kb)
	}

	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Callback(userID, "promo"))
	e.process(t, tgtest.Callback(userID, "menu"))
	if st := e.state(t, userID); st != "await_photo" {
		t.Fatalf("state = %q", st)
	}
}

// Фото в ожидании кода уходит в обычный путь оплаты.
func TestPhotoInPromoState(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Callback(userID, "have_promo:1"))
	e.process(t, tgtest.Photo(userID))
	if n := e.count("copyMessage"); n != 1 {
		t.Fatalf("copyMessage = %d", n)
	}
	if st := e.state(t, userID); st != "in_review" {
		t.Fatalf("state = %q", st)
	}
}
