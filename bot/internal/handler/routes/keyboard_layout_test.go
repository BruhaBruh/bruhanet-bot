package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/texts"
)

// labels — подписи кнопок построчно: «A|B», «C».
func labels(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(dumpKeyboard(t, raw)), "\n") {
		var row []string
		for cell := range strings.SplitSeq(strings.TrimSpace(line), "] [") {
			row = append(row, strings.TrimPrefix(strings.SplitN(cell, " | ", 2)[0], "["))
		}
		out = append(out, strings.Join(row, "|"))
	}
	return out
}

func eqLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLayoutReorderHideTwoColumns(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(20*day))
	e.src.SetLayouts(map[string]texts.Layout{"menu": {
		Rows:   [][]string{{"button.support", "button.pay"}, {"button.gift", "button.referral"}, {"button.info"}},
		Hidden: []string{"button.promo"},
	}})
	e.process(t, tgtest.Message(userID, "/start"))
	// «Подписка» в сохранённой раскладке не упомянута — встала в конец; «Промокод» скрыт
	eqLines(t, labels(t, e.last(t, "sendMessage").Params.Get("reply_markup")),
		[]string{"Поддержка|Оплатить", "Подарить|Пригласить друга", "Информация", "Подписка"})
	// кнопки в очереди (уведомления) берут ту же раскладку
	_, kb := routes.PaymentNotice(t.Context(), e.c, e.rw, userID, "payment.confirmed", now, 0, 0)
	if kb.InlineKeyboard[0][0].Text != "Поддержка" || len(kb.InlineKeyboard) != 4 {
		t.Fatalf("notice keyboard: %+v", kb.InlineKeyboard)
	}
}

// Кнопка, которую код не создал (условие не выполнено), не появляется, сколько бы раскладка её ни требовала.
func TestLayoutDoesNotForceAbsentButton(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(20*day)) // пробный не положен, устройства выключены
	e.src.SetLayouts(map[string]texts.Layout{"menu": {Rows: [][]string{{"button.trial", "button.devices"}, {"button.info"}}}})
	e.process(t, tgtest.Message(userID, "/start"))
	got := labels(t, e.last(t, "sendMessage").Params.Get("reply_markup"))
	if got[0] != "Информация" || strings.Contains(strings.Join(got, ","), "Пробный") || strings.Contains(strings.Join(got, ","), "Устройства") {
		t.Fatalf("got %v", got)
	}
}

func TestLayoutDynamicBlockMoves(t *testing.T) {
	e := newEnv(t)
	e.src.SetLayouts(map[string]texts.Layout{"tariffs": {Rows: [][]string{{"button.back"}, {"@tariffs"}}}})
	e.process(t, tgtest.Callback(userID, "pay"))
	eqLines(t, labels(t, e.last(t, "editMessageText").Params.Get("reply_markup")),
		[]string{"Назад", "1 месяц за 150 ₽|6 месяцев за 800 ₽", "12 месяцев за 1600 ₽"})
}

func TestLayoutConfirmSwapped(t *testing.T) {
	e := newEnv(t)
	e.src.SetLayouts(map[string]texts.Layout{"confirm": {Rows: [][]string{{"button.no", "button.yes"}}, Hidden: []string{"button.have_promo"}}})
	e.process(t, tgtest.Callback(userID, "price:6"))
	eqLines(t, labels(t, e.last(t, "editMessageText").Params.Get("reply_markup")), []string{"Нет|Да"})
}

// Раскладка, не прошедшая проверку, не ломает бота: расположение по умолчанию, предупреждение в логе.
func TestLayoutInvalidFallsBack(t *testing.T) {
	e := newEnv(t)
	e.src.SetLayouts(map[string]texts.Layout{"confirm": {Rows: [][]string{{"button.yes"}}, Hidden: []string{"button.no"}}}) // «Нет» скрывать нельзя
	e.process(t, tgtest.Callback(userID, "price:6"))
	eqLines(t, labels(t, e.last(t, "editMessageText").Params.Get("reply_markup")), []string{"Да|Нет", "Есть промокод"})
	if !strings.Contains(e.log.String(), "invalid keyboard layout") {
		t.Fatalf("no warning: %s", e.log)
	}
}

// Все кнопки скрыты — сообщение уходит без клавиатуры, а не с пустой.
func TestLayoutAllHiddenNoKeyboard(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now)
	_, kb := routes.ReminderMessage(t.Context(), e.c, "day", now, "https://sub")
	if kb == nil {
		t.Fatal("default reminder has no keyboard")
	}
	e.src.SetLayouts(map[string]texts.Layout{"reminder": {Hidden: []string{"button.renew", "button.subscription"}}})
	if _, kb = routes.ReminderMessage(t.Context(), e.c, "day", now, "https://sub"); kb != nil {
		t.Fatalf("keyboard = %+v", kb)
	}
}

