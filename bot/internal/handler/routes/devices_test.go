package routes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
)

const day = 24 * time.Hour

// devicesEnv — продажа устройств включена (60 ₽/мес за устройство), юзер в панели со сроком left.
func devicesEnv(t *testing.T, left time.Duration) *env {
	t.Helper()
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{"devicePrice": []byte("60")})
	e.rw.AddUser(userID, now.Add(left))
	return e
}

func (e *env) text(t *testing.T) string {
	t.Helper()
	return e.last(t, "editMessageText").Params.Get("text")
}

func (e *env) markup(t *testing.T) string {
	t.Helper()
	return e.last(t, "editMessageText").Params.Get("reply_markup")
}

func TestDevicesButtonOnlyWhenEnabled(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(20*day))
	e.process(t, tgtest.Message(userID, "/start"))
	if strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "Устройства") {
		t.Fatal("button shown while the feature is off")
	}
	e.process(t, tgtest.Callback(userID, "devices")) // подделанный callback при выключенной продаже — меню
	if e.count("editMessageText") != 0 && strings.Contains(e.markup(t), "+1") {
		t.Fatal("devices screen opened while off")
	}
	e.src.SetSettings(map[string][]byte{"devicePrice": []byte("60")})
	e.process(t, tgtest.Message(userID, "/start"))
	if !strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "Устройства") {
		t.Fatal("button is missing")
	}
}

func TestDevicesScreenPricesToTermEnd(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetExtraDevices(userID, 2)
	e.process(t, tgtest.Callback(userID, "devices"))
	text, kb := e.text(t), e.markup(t)
	if !strings.Contains(text, "<b>7</b>") || !strings.Contains(text, "60 ₽") {
		t.Fatalf("text: %s", text)
	}
	// 60 * n * 20 / 30
	for _, want := range []string{"+1 — 40 ₽", "+2 — 80 ₽", "+3 — 120 ₽", "+5 — 200 ₽"} {
		if !strings.Contains(kb, want) {
			t.Errorf("no %q in %s", want, kb)
		}
	}
}

func TestDevicesScreenExpired(t *testing.T) {
	e := devicesEnv(t, -time.Hour)
	e.process(t, tgtest.Callback(userID, "devices"))
	if !strings.Contains(e.text(t), "после продления") || strings.Contains(e.markup(t), "dev:") {
		t.Fatalf("%s %s", e.text(t), e.markup(t))
	}
	e.process(t, tgtest.Callback(userID, "devy:1")) // подделка
	if len(e.pay.All()) != 0 {
		t.Fatal("payment created for an expired subscription")
	}
}

func TestDevicesForgedCount(t *testing.T) {
	e := devicesEnv(t, 20*day)
	for _, data := range []string{"devy:4", "devy:0", "devy:-1", "dev:99", "devy:x"} {
		e.process(t, tgtest.Callback(userID, data))
	}
	if len(e.pay.All()) != 0 {
		t.Fatalf("payments: %+v", e.pay.All())
	}
}

func TestDevicesPurchaseFlow(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetExtraDevices(userID, 2)
	e.process(t, tgtest.Callback(userID, "dev:1"))
	if !strings.Contains(e.text(t), "40 ₽") || !strings.Contains(e.text(t), "Срок подписки не изменится") {
		t.Fatalf("confirm: %s", e.text(t))
	}
	e.process(t, tgtest.Callback(userID, "devy:1"))
	e.process(t, tgtest.Callback(userID, "devy:1")) // двойное «Да»
	all := e.pay.All()
	if len(all) != 1 {
		t.Fatalf("payments = %d", len(all))
	}
	p := all[0]
	if p.Kind != model.KindDevices || p.ExtraDevices != 1 || p.Month != 0 || p.Price != 40 || p.PromoID != nil {
		t.Fatalf("payment: %+v", p)
	}
	if e.state(t, userID) != "await_photo" || !strings.Contains(e.text(t), "Оплата устройств") {
		t.Fatalf("state %s text %s", e.state(t, userID), e.text(t))
	}

	e.process(t, tgtest.Photo(userID))
	if len(e.rw.Updates()) != 0 || e.pending(t, userID).AdvanceGranted {
		t.Fatalf("advance for devices: %v", e.rw.Updates())
	}
	if !strings.Contains(e.last(t, "sendMessage").Params.Get("text"), "Фото получено") {
		t.Fatal("no photo ack")
	}

	e.process(t, confirmPress(p.ID))
	if len(e.rw.Updates()) != 0 {
		t.Fatalf("term written for devices: %v", e.rw.Updates())
	}
	if w := e.rw.DeviceLimitWrites(); len(w) != 1 || w[0].Limit != 8 {
		t.Fatalf("limit writes: %+v", w)
	}
	if n, _ := e.pay.ExtraDevices(t.Context(), userID); n != 3 {
		t.Fatalf("extra = %d", n)
	}
	out := e.pay.Outbox()
	if len(out) != 1 || out[0].TextKey != "payment.devices_confirmed" || out[0].Devices != 1 {
		t.Fatalf("outbox: %+v", out)
	}
	e.process(t, confirmPress(p.ID)) // повторное нажатие
	if len(e.rw.DeviceLimitWrites()) != 1 || e.answer(t) != "Уже обработано" {
		t.Fatalf("second press: %q", e.answer(t))
	}
}

func TestDevicesDeclineChangesNothing(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.process(t, tgtest.Callback(userID, "devy:2"))
	p := e.pending(t, userID)
	e.process(t, tgtest.Photo(userID))
	e.process(t, declinePress(p.ID))
	if len(e.rw.Updates()) != 0 || len(e.rw.DeviceLimitWrites()) != 0 {
		t.Fatal("panel touched by a declined devices payment")
	}
	if n, _ := e.pay.ExtraDevices(t.Context(), userID); n != 0 {
		t.Fatalf("extra = %d", n)
	}
	if out := e.pay.Outbox(); len(out) != 1 || out[0].TextKey != "payment.devices_declined" {
		t.Fatalf("outbox: %+v", out)
	}
}

func TestDevicesRefusedWhileTariffPending(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	tariff := e.pending(t, userID)
	e.process(t, tgtest.Callback(userID, "devy:1"))
	if !strings.Contains(e.text(t), "Сначала заверши текущую оплату") {
		t.Fatalf("text: %s", e.text(t))
	}
	if got := e.pending(t, userID); got.ID != tariff.ID || len(e.pay.All()) != 1 {
		t.Fatalf("pending replaced: %+v", e.pay.All())
	}
}

func TestTariffPriceIncludesExtraDevices(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetExtraDevices(userID, 2)
	e.process(t, tgtest.Callback(userID, "price:6"))
	// 800 + 2 * 60 * 6 = 1520
	if !strings.Contains(e.text(t), "1520 ₽") || !strings.Contains(e.text(t), "2 × 60 ₽ × 6 = 720 ₽") ||
		!strings.Contains(e.markup(t), "Без доп. устройств") {
		t.Fatalf("%s\n%s", e.text(t), e.markup(t))
	}
	e.process(t, tgtest.Callback(userID, "buy:6"))
	p := e.pending(t, userID)
	if p.Kind != model.KindTariff || p.Price != 1520 || p.ExtraDevices != 2 {
		t.Fatalf("payment: %+v", p)
	}
	// подтверждение: число не изменилось — лимит в панели не пишется, срок продлён
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(p.ID))
	if len(e.rw.DeviceLimitWrites()) != 0 {
		t.Fatalf("limit written without change: %+v", e.rw.DeviceLimitWrites())
	}
	if want := now.Add(20*day + 6*30*day); !e.rw.ExpireAt(userID).Equal(want) {
		t.Fatalf("expire = %v, want %v", e.rw.ExpireAt(userID), want)
	}
}

func TestTariffDropDevices(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetExtraDevices(userID, 2)
	e.process(t, tgtest.Callback(userID, "buyd:6"))
	p := e.pending(t, userID)
	if p.Price != 800 || p.ExtraDevices != 0 {
		t.Fatalf("payment: %+v", p)
	}
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(p.ID))
	if w := e.rw.DeviceLimitWrites(); len(w) != 1 || w[0].Limit != 5 {
		t.Fatalf("limit writes: %+v", w)
	}
	if n, _ := e.pay.ExtraDevices(t.Context(), userID); n != 0 {
		t.Fatalf("extra = %d", n)
	}
}

// Аванс (фото чека) продлевает срок на день, лимит устройств не трогает.
func TestAdvanceKeepsDeviceLimit(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetExtraDevices(userID, 2)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	e.process(t, tgtest.Photo(userID))
	if !e.pending(t, userID).AdvanceGranted || len(e.rw.Updates()) != 1 {
		t.Fatalf("advance not granted: %v", e.rw.Updates())
	}
	if len(e.rw.DeviceLimitWrites()) != 0 {
		t.Fatalf("limit written by advance: %+v", e.rw.DeviceLimitWrites())
	}
}

func TestNewUserGetsBaseLimit(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if w := e.rw.DeviceLimitWrites(); len(w) != 1 || w[0].Limit != 5 || w[0].TelegramID != userID {
		t.Fatalf("limit writes: %+v", w)
	}
}

func TestBlockedUserMayBuyDevices(t *testing.T) {
	e := devicesEnv(t, 20*day)
	e.pay.SetBlocked(userID, true)
	e.process(t, tgtest.Callback(userID, "devy:1"))
	if p := e.pending(t, userID); p.Kind != model.KindDevices {
		t.Fatalf("payment: %+v", p)
	}
}
