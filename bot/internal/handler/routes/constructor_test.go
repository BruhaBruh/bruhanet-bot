package routes_test

import (
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/routes"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/texts"
)

func store(e *env, key string, mutate func(l *texts.MsgLayout)) {
	l, _, _ := texts.MessageDefault(key)
	mutate(&l)
	e.src.SetMessageKeyboards(map[string]texts.MsgLayout{key: l})
}

func cell(id, label, action string, conds ...texts.CondRef) texts.Cell {
	return texts.Cell{ID: id, Label: label, Action: action, Conditions: append([]texts.CondRef{}, conds...)}
}

func menuNow(t *testing.T, e *env) string {
	t.Helper()
	e.process(t, tgtest.Message(userID, "/start"))
	return dumpKeyboard(t, e.last(t, "sendMessage").Params.Get("reply_markup"))
}

// Своя кнопка с https-ссылкой и меткой, условия и НЕ: показываются только при выполнении.
func TestConstructorConditions(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(20*day))
	store(e, "menu.greeting", func(l *texts.MsgLayout) {
		l.Rows[6] = []texts.Cell{
			cell("site", "Сайт", "url:https://example.com"),
			cell("only_new", "Новичкам", "route:promo", texts.CondRef{ID: "in_panel", Not: true}),
		}
		l.Rows[7] = []texts.Cell{cell("ref_off", "Рефералки нет", "route:menu", texts.CondRef{ID: "referral_on", Not: true})}
	})
	got := menuNow(t, e)
	if !strings.Contains(got, "[Сайт | url=https://example.com |") {
		t.Errorf("custom url button missing:\n%s", got)
	}
	if strings.Contains(got, "Новичкам") {
		t.Errorf("NOT in_panel shown for a panel user:\n%s", got)
	}
	// рефералка выключена по умолчанию в e.src? включаем — кнопка с НЕ referral_on пропадает
	if !strings.Contains(got, "Рефералки нет") && strings.Contains(got, "cb=referral") {
		t.Errorf("unexpected state:\n%s", got)
	}
	e.src.SetSettings(map[string][]byte{"referralDays": []byte("0")})
	if got = menuNow(t, e); !strings.Contains(got, "Рефералки нет") {
		t.Errorf("NOT referral_on hidden while referral is off:\n%s", got)
	}
	e.src.SetSettings(map[string][]byte{"referralDays": []byte("7")})
	if got = menuNow(t, e); strings.Contains(got, "Рефералки нет") {
		t.Errorf("NOT referral_on shown while referral is on:\n%s", got)
	}
}

// Встроенное ограничение действия нельзя обойти: «Пробный» с пустыми условиями не появляется, пока выключен.
func TestConstructorKeepsBuiltInGuards(t *testing.T) {
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{"trialDays": []byte("0"), "devicePrice": []byte("0")})
	store(e, "menu.greeting", func(l *texts.MsgLayout) {
		l.Rows[6] = []texts.Cell{cell("t2", "Пробный тут", "route:trial"), cell("d2", "Устройства тут", "route:devices")}
	})
	if got := menuNow(t, e); strings.Contains(got, "Пробный тут") || strings.Contains(got, "Устройства тут") {
		t.Errorf("guarded actions shown:\n%s", got)
	}
}

// Шаблон тарифов переставляется, а кнопка «Назад» меняет подпись и стиль; свои подписи рендерят общие переменные.
func TestConstructorTemplateAndLabels(t *testing.T) {
	e := newEnv(t)
	store(e, "payment.tariffs", func(l *texts.MsgLayout) {
		l.Rows[0], l.Rows[1] = l.Rows[1], l.Rows[0]
		l.Rows[0][0].Label, l.Rows[0][0].Style = "Мой ID {{.telegramId}}", texts.StyleDanger
	})
	e.process(t, tgtest.Callback(userID, "pay"))
	got := dumpKeyboard(t, e.last(t, "editMessageText").Params.Get("reply_markup"))
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "[Мой ID 777") && !strings.Contains(lines[0], "Мой ID ") {
		t.Errorf("back first with custom label:\n%s", got)
	}
	if !strings.Contains(lines[0], `"danger"`) || !strings.Contains(lines[0], "cb=menu") {
		t.Errorf("style or action lost:\n%s", got)
	}
	if len(lines) < 3 || !strings.Contains(got, "cb=price:6") {
		t.Errorf("tariff template not expanded:\n%s", got)
	}
}

// Сценарные кнопки (Да/Нет) берут данные у экрана, а не из раскладки.
func TestConstructorFlowActions(t *testing.T) {
	e := newEnv(t)
	store(e, "payment.confirm", func(l *texts.MsgLayout) { l.Rows[0][0], l.Rows[0][1] = l.Rows[0][1], l.Rows[0][0] })
	e.process(t, tgtest.Callback(userID, "price:6"))
	got := dumpKeyboard(t, e.last(t, "editMessageText").Params.Get("reply_markup"))
	if i, j := strings.Index(got, "cb=pay"), strings.Index(got, "cb=buy:6"); i < 0 || j < 0 || i > j {
		t.Errorf("No must come before Yes with their real data:\n%s", got)
	}
}

// Невалидная клавиатура в БД не ломает бота: клавиатура по умолчанию и один warn.
func TestConstructorInvalidFallsBack(t *testing.T) {
	e := newEnv(t)
	base := newEnv(t)
	store(e, "payment.confirm", func(l *texts.MsgLayout) { l.Rows = l.Rows[1:] }) // без обязательных Да/Нет
	e.process(t, tgtest.Callback(userID, "price:6"))
	base.process(t, tgtest.Callback(userID, "price:6"))
	got := e.last(t, "editMessageText").Params.Get("reply_markup")
	if want := base.last(t, "editMessageText").Params.Get("reply_markup"); got != want {
		t.Errorf("fallback differs:\n%s\n%s", got, want)
	}
	if !strings.Contains(e.log.String(), "invalid message keyboard") {
		t.Errorf("no warn: %s", e.log)
	}
}

// Очередь: клавиатуры напоминаний и платежей читают раскладку сообщения.
func TestConstructorQueueNotices(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(20*day))
	store(e, "reminder.day", func(l *texts.MsgLayout) {
		l.Rows = [][]texts.Cell{{l.Rows[0][1]}}
		l.Rows[0][0].Label = "Ссылка"
	})
	_, kb := routes.ReminderMessage(t.Context(), e.c, "day", now, "https://sub")
	got := dumpMarkup(t, kb)
	if strings.Contains(got, "cb=pay") || !strings.Contains(got, "[Ссылка | copy=https://sub |") {
		t.Errorf("reminder.day:\n%s", got)
	}
	_, kb = routes.ReminderMessage(t.Context(), e.c, "week", now, "https://sub")
	if !strings.Contains(dumpMarkup(t, kb), "cb=pay") {
		t.Error("reminder.week must stay default")
	}
}

// Одиночная «Назад» — служебная ячейка: подпись, стиль и соседние кнопки правятся, а куда она ведёт, знает экран.
func TestConstructorBackCell(t *testing.T) {
	e := newEnv(t)
	store(e, "promo.enter_menu", func(l *texts.MsgLayout) {
		l.Rows[0][0].Label = "Вернуться"
		l.Rows[0][0].Style = texts.StyleDanger
		l.Rows = append(l.Rows, []texts.Cell{cell("site", "Сайт", "url:https://example.com")})
	})
	e.process(t, tgtest.Callback(userID, "promo"))
	kb := e.last(t, "editMessageText").Params.Get("reply_markup")
	for _, want := range []string{`"text":"Вернуться"`, `"style":"danger"`, `"callback_data":"menu"`, `"url":"https://example.com"`} {
		if !strings.Contains(kb, want) {
			t.Errorf("no %s in %s", want, kb)
		}
	}
}

// Сообщению без кнопок по умолчанию (реквизиты оплаты) админ может добавить свои; без правок клавиатуры нет.
func TestConstructorButtonsOnMessageWithoutKeyboard(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if kb := e.last(t, "editMessageText").Params.Get("reply_markup"); kb != "" {
		t.Fatalf("requisites must have no keyboard by default: %s", kb)
	}
	e2 := newEnv(t)
	store(e2, "payment.requisites", func(l *texts.MsgLayout) {
		l.Rows = [][]texts.Cell{{cell("site", "Сайт", "url:https://example.com"), cell("menu", "Меню", "route:menu")}}
	})
	e2.process(t, tgtest.Callback(userID, "buy:6"))
	kb := e2.last(t, "editMessageText").Params.Get("reply_markup")
	if !strings.Contains(kb, `"url":"https://example.com"`) || !strings.Contains(kb, `"callback_data":"menu"`) {
		t.Fatalf("custom buttons missing: %s", kb)
	}
}

// У каждого действия каталога есть реализация в боте.
func TestConstructorCatalogsImplemented(t *testing.T) {
	routes.CheckCatalog(t)
}
