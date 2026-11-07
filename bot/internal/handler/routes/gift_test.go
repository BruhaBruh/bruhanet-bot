package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
)

const (
	giftCode       = "AbCdEfGhIjKlMnOpQrStUv"
	buyerID  int64 = 777
)

func (e *env) giftText(t *testing.T) string {
	t.Helper()
	return e.last(t, "sendMessage").Params.Get("text")
}

// Покупка: тариф → «Верно?» → «Да» → реквизиты → чек → админ подтверждает: ссылка покупателю, его срок и панель не тронуты.
func TestGiftBuyFlow(t *testing.T) {
	e := newEnv(t)
	term := now.Add(10 * 24 * time.Hour)
	e.rw.AddUser(userID, term)

	e.process(t, tgtest.Callback(userID, "gift"))
	kb := e.last(t, "editMessageText").Params.Get("reply_markup")
	if !strings.Contains(kb, "gprice:6") || strings.Contains(kb, `"price:6"`) {
		t.Fatalf("tariffs keyboard: %s", kb)
	}
	e.process(t, tgtest.Callback(userID, "gprice:6"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "6 месяцев") || !strings.Contains(text, "800 ₽") ||
		!strings.Contains(text, "не изменится") {
		t.Fatalf("confirm: %s", text)
	}
	e.process(t, tgtest.Callback(userID, "gbuy:6"))
	p := e.pending(t, userID)
	if p.Kind != model.KindGift || p.Month != 6 || p.Price != 800 || p.PromoID != nil || p.ExtraDevices != 0 {
		t.Fatalf("payment = %+v", p)
	}
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "Оплата подарка: 6 месяцев за 800 ₽") {
		t.Fatalf("requisites: %s", text)
	}
	// двойное «Да» не плодит платежи
	e.process(t, tgtest.Callback(userID, "gbuy:6"))
	if len(e.pay.All()) != 1 {
		t.Fatalf("payments = %d", len(e.pay.All()))
	}

	e.process(t, tgtest.Photo(userID))
	if text := e.lastGiftReview(t); !strings.Contains(text, "Покупка подарка") || !strings.Contains(text, "Срок покупателя не меняется") {
		t.Fatalf("review: %s", text)
	}
	if len(e.rw.Updates()) != 0 { // ни аванса, ни продления
		t.Fatalf("panel updates: %+v", e.rw.Updates())
	}

	e.process(t, confirmPress(p.ID))
	gifts := e.pay.Gifts()
	if len(gifts) != 1 || gifts[0].Status != model.GiftReady || gifts[0].Months != 6 {
		t.Fatalf("gifts = %+v", gifts)
	}
	out := e.pay.Outbox()
	if len(out) != 1 || out[0].TextKey != "gift.ready" || out[0].GiftCode != gifts[0].Code || out[0].ChatID != userID || out[0].Months != 6 {
		t.Fatalf("outbox = %+v", out)
	}
	if len(e.rw.Updates()) != 0 || !e.rw.ExpireAt(userID).Equal(term) || len(e.rw.DeviceLimitWrites()) != 0 {
		t.Fatal("buyer's term or devices changed")
	}
	// уведомление: ссылка в тексте и кнопка копирования
	text, markup := routes.GiftNotice(t.Context(), e.c, e.rw, "bruhabot", out[0])
	link := "https://t.me/bruhabot?start=gift_" + gifts[0].Code
	if !strings.Contains(text, link) || !strings.Contains(text, "6 месяцев") {
		t.Fatalf("notice: %s", text)
	}
	copied := false
	for _, row := range markup.InlineKeyboard {
		for _, b := range row {
			copied = copied || (b.CopyText != nil && b.CopyText.Text == link)
		}
	}
	if !copied {
		t.Fatalf("no copy button: %+v", markup)
	}

	// отклонение гасит подарок и пишет отказ без ссылки
	e2 := newEnv(t)
	e2.rw.AddUser(userID, term)
	e2.process(t, tgtest.Callback(userID, "gbuy:1"))
	e2.process(t, tgtest.Photo(userID))
	e2.process(t, declinePress(e2.pending(t, userID).ID))
	if g := e2.pay.Gifts(); len(g) != 1 || g[0].Status != model.GiftCancelled {
		t.Fatalf("gifts = %+v", g)
	}
	if out := e2.pay.Outbox(); len(out) != 1 || out[0].TextKey != "gift.declined" || out[0].GiftCode != "" {
		t.Fatalf("outbox = %+v", out)
	}
}

func (e *env) lastGiftReview(t *testing.T) string {
	t.Helper()
	var text string
	for _, c := range e.srv.Calls("sendMessage") {
		if c.Params.Get("chat_id") == "1" && strings.Contains(c.Params.Get("text"), "Покупка подарка") {
			text = c.Params.Get("text")
		}
	}
	return text
}

// Подделанный срок возвращает к списку; при открытой оплате подарок не создаётся и чужой платёж не отменяется.
func TestGiftBuyRejections(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "gprice:5"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "На сколько подарим подписку") {
		t.Fatalf("forged price: %s", text)
	}
	e.process(t, tgtest.Callback(userID, "gbuy:99"))
	if len(e.pay.All()) != 0 {
		t.Fatal("payment for a forged tariff")
	}

	e.process(t, tgtest.Callback(userID, "buy:6"))
	tariff := e.pending(t, userID)
	e.process(t, tgtest.Callback(userID, "gbuy:6"))
	if text := e.last(t, "editMessageText").Params.Get("text"); !strings.Contains(text, "заверши текущую оплату") {
		t.Fatalf("busy: %s", text)
	}
	if p := e.pending(t, userID); p.ID != tariff.ID || p.Kind != model.KindTariff || len(e.pay.Gifts()) != 0 {
		t.Fatalf("pending replaced: %+v", p)
	}
}

// Ссылка: срок от max(панель, now) + месяцы·30 суток, уведомления через outbox, один раз; все отказы одинаковы.
func TestRedeemGiftByLink(t *testing.T) {
	e := newEnv(t)
	e.pay.SeedGift(giftCode, 3, buyerID, model.GiftReady)

	e.process(t, tgtest.Message(userID, "/start gift_"+giftCode))
	if want := now.Add(90 * 24 * time.Hour); !e.rw.ExpireAt(userID).Equal(want) {
		t.Fatalf("expire = %v, want %v", e.rw.ExpireAt(userID), want)
	}
	if g := e.pay.Gifts(); g[0].Status != model.GiftRedeemed || g[0].RedeemedBy == nil || *g[0].RedeemedBy != userID {
		t.Fatalf("gift = %+v", g[0])
	}
	out := e.pay.Outbox()
	if len(out) != 2 || out[0].ChatID != userID || out[0].TextKey != "gift.redeemed" || out[1].ChatID != buyerID || out[1].TextKey != "gift.redeemed_buyer" {
		t.Fatalf("outbox = %+v", out)
	}
	// покупателю не уходит ничего о получателе
	buyerText, _ := routes.GiftNotice(t.Context(), e.c, e.rw, "bot", out[1])
	if strings.Contains(buyerText, "42") || strings.Contains(buyerText, "Привет") {
		t.Fatalf("buyer notice leaks the redeemer: %s", buyerText)
	}
	redeemerText, _ := routes.GiftNotice(t.Context(), e.c, e.rw, "bot", out[0])
	if !strings.Contains(redeemerText, "3 месяца") || !strings.Contains(redeemerText, "Действует до") {
		t.Fatalf("redeemer notice: %s", redeemerText)
	}
	if e.pay.Grants()[len(e.pay.Grants())-1].Source != "gift" {
		t.Fatalf("grants = %+v", e.pay.Grants())
	}

	// повтор той же ссылки
	before := len(e.rw.Updates())
	e.process(t, tgtest.Message(userID, "/start gift_"+giftCode))
	repeat := e.giftText(t)
	if !strings.Contains(repeat, "недоступен") || len(e.rw.Updates()) != before {
		t.Fatalf("repeat: %s", repeat)
	}

	// все остальные отказы дают тот же текст
	e.pay.SeedGift("CancelledCancelled1", 1, 0, model.GiftCancelled)
	e.pay.SeedGift("PendingPendingPend1", 1, buyerID, model.GiftPending)
	e.pay.SeedGift("BlockedBlockedBlock1", 1, 0, model.GiftReady)
	e.pay.SetBlocked(userID, true)
	for _, payload := range []string{"CancelledCancelled1", "PendingPendingPend1", "BlockedBlockedBlock1", "NoSuchNoSuchNoSuch12", "x", "bad!bad!bad!bad!bad!"} {
		e.process(t, tgtest.Message(userID, "/start gift_"+payload))
		if got := e.giftText(t); got != repeat {
			t.Errorf("%s: %q != %q", payload, got, repeat)
		}
	}
	e.pay.SetBlocked(userID, false)
	if g := e.pay.Gifts(); g[3].Status != model.GiftReady {
		t.Fatalf("blocked user's refusal changed the gift: %+v", g[3])
	}
}

// Новый юзер: заводится в панели и получает срок; лимит устройств — база из настроек.
func TestRedeemGiftNewUser(t *testing.T) {
	e := newEnv(t)
	e.pay.SeedGift(giftCode, 1, 0, model.GiftReady) // выдан админом: покупателя нет
	e.process(t, tgtest.Message(userID, "/start gift_"+giftCode))
	if want := now.Add(30 * 24 * time.Hour); !e.rw.ExpireAt(userID).Equal(want) {
		t.Fatalf("expire = %v, want %v", e.rw.ExpireAt(userID), want)
	}
	if out := e.pay.Outbox(); len(out) != 1 || out[0].TextKey != "gift.redeemed" {
		t.Fatalf("outbox = %+v", out)
	}
}

// Ошибка панели: подарок остаётся, юзеру общая ошибка, повтор той же ссылкой работает.
func TestRedeemGiftPanelFailure(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	e.pay.SeedGift(giftCode, 1, 0, model.GiftReady)
	e.rw.FailUpdateWith(errors.New("panel is down"))
	e.process(t, tgtest.Message(userID, "/start gift_"+giftCode))
	if g := e.pay.Gifts(); g[0].Status != model.GiftReady || len(e.pay.Outbox()) != 0 {
		t.Fatalf("not rolled back: %+v", g[0])
	}
	if text := e.giftText(t); !strings.Contains(text, "Что-то пошло не так") {
		t.Fatalf("text: %s", text)
	}
	e.rw.FailUpdateWith(nil)
	e.process(t, tgtest.Message(userID, "/start gift_"+giftCode))
	if g := e.pay.Gifts(); g[0].Status != model.GiftRedeemed {
		t.Fatalf("retry failed: %+v", g[0])
	}
}

// Покупателю при отзыве подарка приходит gift.cancelled с главным меню.
func TestGiftCancelledNotice(t *testing.T) {
	e := newEnv(t)
	text, kb := routes.GiftNotice(t.Context(), e.c, e.rw, "bot", model.OutboxMessage{ChatID: userID, TextKey: "gift.cancelled", Months: 6})
	if !strings.Contains(text, "Подарок отозван") || !strings.Contains(text, "6 месяцев") || kb == nil {
		t.Fatalf("text = %q kb = %v", text, kb)
	}
}
