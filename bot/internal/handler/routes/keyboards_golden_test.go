package routes_test

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
)

var update = flag.Bool("update", false, "rewrite testdata/keyboards.golden")

// dumpKeyboard — разметка как её видит юзер: строки, подпись, действие и стиль каждой кнопки.
func dumpKeyboard(t *testing.T, raw string) string {
	t.Helper()
	var kb models.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(raw), &kb); err != nil {
		t.Fatalf("reply_markup %q: %v", raw, err)
	}
	var sb strings.Builder
	for _, row := range kb.InlineKeyboard {
		cells := make([]string, 0, len(row))
		for _, b := range row {
			var act string
			switch {
			case b.URL != "":
				act = "url=" + b.URL
			case b.CopyText != nil:
				act = "copy=" + b.CopyText.Text
			default:
				act = "cb=" + b.CallbackData
			}
			cells = append(cells, fmt.Sprintf("[%s | %s | %q]", b.Text, act, b.Style))
		}
		sb.WriteString("  " + strings.Join(cells, " ") + "\n")
	}
	if sb.Len() == 0 {
		return "  (empty)\n"
	}
	return sb.String()
}

func dumpMarkup(t *testing.T, kb *models.InlineKeyboardMarkup) string {
	t.Helper()
	raw, err := json.Marshal(kb)
	if err != nil {
		t.Fatal(err)
	}
	return dumpKeyboard(t, string(raw))
}

// Раскладка клавиатур без единой правки из админки — побайтно как до появления настраиваемых раскладок.
func TestKeyboardsGolden(t *testing.T) {
	goldenCheck(t, true)
}

// Клавиатура по умолчанию из конструктора, «сохранённая» у каждого сообщения, даёт те же 41 клавиатуру побайтно:
// путь конструктора (условия, действия, шаблоны) равен коду.
func TestKeyboardsGoldenViaConstructor(t *testing.T) {
	defaultsAsStored = true
	t.Cleanup(func() { defaultsAsStored = false })
	goldenCheck(t, false)
}

func goldenCheck(t *testing.T, canUpdate bool) {
	t.Helper()
	var out strings.Builder
	add := func(name, dump string) { out.WriteString("== " + name + "\n" + dump) }
	sent := func(e *env) string { return dumpKeyboard(t, e.last(t, "sendMessage").Params.Get("reply_markup")) }
	edited := func(e *env) string { return dumpKeyboard(t, e.last(t, "editMessageText").Params.Get("reply_markup")) }
	scenario := func(name string, setup func(e *env), steps func(e *env) string) {
		e := newEnv(t)
		if setup != nil {
			setup(e)
		}
		add(name, steps(e))
	}
	devices := func(e *env) { e.src.SetSettings(map[string][]byte{"devicePrice": []byte("60")}) }
	start := func(e *env) string { e.process(t, tgtest.Message(userID, "/start")); return sent(e) }
	cb := func(data string) func(e *env) string {
		return func(e *env) string { e.process(t, tgtest.Callback(userID, data)); return edited(e) }
	}

	scenario("menu: not in panel, trial+referral on", nil, start)
	scenario("menu: not in panel, trial+referral off", func(e *env) {
		e.src.SetSettings(map[string][]byte{"trialDays": []byte("0"), "referralDays": []byte("0")})
	}, start)
	scenario("menu: in panel, devices off", func(e *env) { e.rw.AddUser(userID, now.Add(20*day)) }, start)
	scenario("menu: in panel, devices on, trial not offered", func(e *env) {
		devices(e)
		e.rw.AddUser(userID, now.Add(20*day))
	}, start)
	scenario("menu: in panel, expired (trial offered)", func(e *env) {
		devices(e)
		e.rw.AddUser(userID, now.Add(-time.Hour))
	}, start)
	scenario("menu: in panel, everything off", func(e *env) {
		e.src.SetSettings(map[string][]byte{"trialDays": []byte("0"), "referralDays": []byte("0")})
		e.rw.AddUser(userID, now.Add(20*day))
	}, start)
	scenario("menu: panel down", func(e *env) { e.rw.FailWith(errors.New("panel down")) }, start)
	scenario("info: not bought", nil, cb("info"))
	scenario("info: bought", func(e *env) { e.rw.AddUser(userID, now.Add(20*day)) }, cb("info"))
	scenario("error.generic", func(e *env) { e.rw.AddUser(userID, now.Add(20*day)); e.rw.FailWith(errors.New("x")) }, cb("info"))
	scenario("tariffs", nil, cb("pay"))
	scenario("tariffs: one tariff", func(e *env) {
		e.src.SetSettings(map[string][]byte{"prices": []byte(`{"3":300}`)})
	}, cb("pay"))
	scenario("tariffs: odd count", func(e *env) {
		e.src.SetSettings(map[string][]byte{"prices": []byte(`{"1":100,"3":300,"6":500,"9":700,"12":900}`)})
	}, cb("pay"))
	scenario("payment.confirm", nil, cb("price:6"))
	scenario("payment.confirm_devices", func(e *env) {
		devices(e)
		e.rw.AddUser(userID, now.Add(10*day))
		e.pay.SetExtraDevices(userID, 1)
	}, cb("price:6"))
	scenario("promo.enter_payment", nil, cb("have_promo:6"))
	scenario("promo.not_found", nil, func(e *env) string {
		e.process(t, tgtest.Callback(userID, "have_promo:6"))
		e.process(t, tgtest.Message(userID, "NOPE"))
		return sent(e)
	})
	scenario("promo.enter_menu", func(e *env) { e.rw.AddUser(userID, now) }, cb("promo"))
	scenario("promo.confirm_discount", func(e *env) {
		e.pay.AddPromo(model.Promo{Code: "SUMMER25", Kind: model.PromoPercent, Value: 25})
	}, func(e *env) string {
		e.process(t, tgtest.Callback(userID, "have_promo:1"))
		e.process(t, tgtest.Message(userID, "summer25"))
		return sent(e)
	})
	scenario("devices.menu", func(e *env) { devices(e); e.rw.AddUser(userID, now.Add(20*day)) }, cb("devices"))
	scenario("devices.no_term", func(e *env) { devices(e); e.rw.AddUser(userID, now.Add(-time.Hour)) }, cb("devices"))
	scenario("devices.confirm", func(e *env) { devices(e); e.rw.AddUser(userID, now.Add(20*day)) }, cb("dev:2"))
	scenario("devices.busy", func(e *env) {
		devices(e)
		e.rw.AddUser(userID, now.Add(20*day))
		e.process(t, tgtest.Callback(userID, "buy:6"))
	}, cb("devy:2"))
	scenario("gift.tariffs", nil, cb("gift"))
	scenario("gift.confirm", nil, cb("gprice:6"))
	scenario("gift.busy", func(e *env) { e.process(t, tgtest.Callback(userID, "buy:6")) }, cb("gbuy:6"))
	scenario("gift.unavailable", nil, func(e *env) string {
		e.process(t, tgtest.Message(userID, "/start gift_AAAAAAAAAAAAAAAAAAAAAA"))
		return sent(e)
	})
	scenario("trial.confirm", nil, cb("trial"))
	scenario("referral.menu", nil, cb("referral"))

	// уведомления из очереди
	c := newEnv(t)
	c.rw.AddUser(userID, now.Add(20*day))
	for _, kind := range []string{"day", "week"} {
		_, kb := routes.ReminderMessage(t.Context(), c.c, kind, now, "https://sub")
		add("reminder."+kind, dumpMarkup(t, kb))
	}
	_, kb := routes.GiftNotice(t.Context(), c.c, c.rw, "bruhabot", model.OutboxMessage{ChatID: userID, TextKey: "gift.ready", GiftCode: "CODE", Months: 6})
	add("gift.ready", dumpMarkup(t, kb))
	_, kb = routes.GiftNotice(t.Context(), c.c, c.rw, "bruhabot", model.OutboxMessage{ChatID: userID, TextKey: "gift.declined"})
	add("gift.declined", dumpMarkup(t, kb))
	_, kb = routes.PaymentNotice(t.Context(), c.c, c.rw, userID, "payment.confirmed", now, 0, 0)
	add("payment.confirmed", dumpMarkup(t, kb))
	n := newEnv(t)
	_, kb = routes.PaymentNotice(t.Context(), n.c, n.rw, userID, "payment.declined", now, 0, 0)
	add("payment.declined (not in panel)", dumpMarkup(t, kb))

	const path = "testdata/keyboards.golden"
	if *update && canUpdate {
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != string(want) {
		t.Fatalf("keyboards changed (run with -update only for an intended change)\n--- got\n%s\n--- want\n%s", got, want)
	}
}
