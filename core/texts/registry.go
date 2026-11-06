// Package texts — реестр текстов бота, которые видит пользователь: значения по умолчанию,
// рендер {{.path}} в Telegram HTML и проверка правок.
package texts

import (
	"maps"
	"slices"
	"unicode/utf16"
)

type Kind string

const (
	KindMessage Kind = "message"
	KindButton  Kind = "button"
	// KindPlain — простой текст-значение без разметки (подставляется в другие тексты).
	KindPlain Kind = "plain"
)

// Var — переменная текста. Raw — значение уже готовый HTML и не экранируется (RenderRaw).
// Max — наибольшая длина значения (видимые UTF-16 units): Validate проверяет лимит Telegram по худшему случаю.
type Var struct {
	Name, Description, Sample string
	Raw                       bool
	Max                       int
}

type Def struct {
	Key, Group, Title string
	Kind              Kind
	Default           string
	Vars              []Var
	When              string   // когда и кому отправляется, для админа
	Buttons           []string // ключи кнопок под сообщением, как их показывает бот
	Route             string   // кнопка: действие в коде (callback-роут, url:… или copy:…)
	Style             string   // кнопка: вид на основном экране, пока админ не сохранил свой
}

// Стили кнопок — значения keyboard.Style* (core не зависит от pkg; совпадение проверяет тест бота).
const (
	StyleDefault = ""
	StylePrimary = "primary"
	StyleSuccess = "success"
	StyleDanger  = "danger"
)

// Styles — допустимые стили кнопки.
var Styles = []string{StyleDefault, StylePrimary, StyleSuccess, StyleDanger}

const (
	groupMenu     = "Меню"
	groupPayment  = "Оплата"
	groupPromo    = "Промокоды"
	groupReminder = "Напоминания"
	groupError    = "Ошибки"
	groupButton   = "Кнопки"
)

// MaxCustomVar — наибольшая длина значения своей переменной (core/settings), UTF-16.
const MaxCustomVar = 96

// globals — общие переменные: есть в каждом тексте любого вида. Значения подставляет вызывающий
// (бот — content.Provider, на примерах — Samples). С переменными текстов имена не пересекаются (тест реестра).
var globals = []Var{
	{Name: "name", Description: "Имя пользователя в Telegram", Sample: "Иван", Max: 64},
	{Name: "username", Description: "Username в Telegram без @; нет — «—»", Sample: "ivan", Max: 32},
	{Name: "telegramId", Description: "Telegram ID пользователя", Sample: "123456789", Max: 20},
	{Name: "expireAt", Description: "Дата окончания подписки (МСК); нет — «—»", Sample: "2 ноября 2026 г. в 15:04, MSK", Max: 40},
	{Name: "phone", Description: "Номер для перевода из настроек", Sample: "+79990000000", Max: 64},
	{Name: "bank", Description: "Банк для перевода из настроек", Sample: "Т-Банк", Max: 64},
	{Name: "supportUrl", Description: "Ссылка на поддержку из настроек", Sample: "https://t.me/support", Max: 256},
}

var (
	varMonths = Var{Name: "months", Description: "Срок тарифа с единицей: «3 месяца»", Sample: "3 месяца", Max: 16}
	varPrice  = Var{Name: "price", Description: "Цена тарифа в рублях", Sample: "500", Max: 8}
	varCode   = Var{Name: "code", Description: "Промокод", Sample: "SUMMER25", Max: 32}
)

// maxComments — не больше комментариев к переводу (core/settings).
const maxComments = 10

const (
	photoSent    = "<b>Вы успешно отправили фото на проверку</b>"
	reminderHead = "<b>Уведомление об окончании подписки</b>\n\n"
	reminderTail = "\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}"
	tariffHead   = "<b>Выбор тарифа</b>\n\n"
	promoHead    = "<b>Промокод</b>\n\n"
	infoHead     = "<b>Информация о подписке</b>\n\n<b>Статус</b>\n"
)

func msg(key, group, title, def string, vars ...Var) Def {
	return Def{Key: key, Group: group, Title: title, Kind: KindMessage, Default: def, Vars: vars}
}

func plain(key, title, def string) Def {
	return Def{Key: key, Group: groupMenu, Title: title, Kind: KindPlain, Default: def}
}

func btn(key, title, def string, vars ...Var) Def {
	return Def{Key: key, Group: groupButton, Title: title, Kind: KindButton, Default: def, Vars: vars}
}

// Имена callback-роутов бота для показа в админке; совпадение с клавиатурами проверяет тест бота.
const (
	routeMenu = "menu"
	routeInfo = "info"
	routePay  = "pay"
)

// shown — когда отправляется и какие кнопки под сообщением.
func (d Def) shown(when string, buttons ...string) Def {
	d.When, d.Buttons = when, buttons
	return d
}

// on — действие кнопки в коде, вид на основном экране и где она показывается.
func (d Def) on(route, style, when string) Def {
	d.Route, d.Style, d.When = route, style, when
	return d
}

// mainButtons — главная клавиатура (routes.mainKeyboard): набор зависит от того, есть ли юзер в панели.
var mainButtons = []string{
	"button.info", "button.pay", "button.buy", "button.subscription", "button.support", "button.promo", "button.service_down",
}

const reasonPromo = "В ответ на присланный промокод: "

var registry = []Def{
	msg("menu.greeting", groupMenu, "Приветствие", "Здравствуйте, <b>{{.name}}</b>").
		shown("На /start, по кнопке «Назад» в меню и на любое сообщение вне сценария оплаты", mainButtons...),
	msg("menu.info", groupMenu, "Информация о подписке",
		infoHead+"{{.status}}\n\n<b>Трафик</b>\n{{.traffic}}\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}\n\n<b>Ваш Telegram ID</b>\n{{.telegramId}}",
		Var{Name: "status", Description: "Статус подписки (тексты «Статус: …»)", Sample: "Активна", Max: 64},
		Var{Name: "traffic", Description: "Использованный трафик из лимита", Sample: "1.50 ГБ из ∞", Max: 32},
	).shown("По кнопке «Информация», если юзер есть в панели", "button.renew", "button.subscription", "button.back"),
	msg("menu.info_not_bought", groupMenu, "Информация: подписка не куплена", infoHead+"Не куплена").
		shown("По кнопке «Информация», если юзера нет в панели", "button.buy", "button.back"),
	plain("menu.status_active", "Статус: активна", "Активна").shown("Подставляется в «Информацию о подписке», когда подписка активна"),
	plain("menu.status_disabled", "Статус: отключена", "Отключена").shown("Подставляется в «Информацию о подписке», когда юзер отключён в панели"),
	plain("menu.status_limited", "Статус: ограничена", "Ограничена").shown("Подставляется в «Информацию о подписке», когда исчерпан лимит трафика"),
	plain("menu.status_expired", "Статус: исчерпана", "Исчерпана").shown("Подставляется в «Информацию о подписке», когда срок истёк"),
	plain("menu.status_unknown", "Статус: неизвестно", "Неизвестно").shown("Подставляется в «Информацию о подписке», когда панель вернула другой статус"),

	msg("payment.tariffs", groupPayment, "Выбор тарифа", tariffHead+"Выберите тариф").
		shown("По кнопкам «Оплатить», «Купить», «Продлить» и «Нет» на подтверждении тарифа", "button.tariff", "button.back"),
	msg("payment.confirm", groupPayment, "Подтверждение тарифа",
		tariffHead+"Вы выбрали тариф на <b>{{.months}}</b> за <b>{{.price}} ₽</b>\n\nВерно?",
		varMonths, varPrice).
		shown("После выбора тарифа в списке, а также по «Убрать промокод»", "button.yes", "button.no", "button.have_promo"),
	msg("payment.requisites", groupPayment, "Реквизиты для оплаты",
		"<b>Оплата тарифа на {{.months}} за {{.price}} ₽</b>\n\n<b>Для оплаты</b>\nПереведите <b>{{.price}} ₽</b>\n"+
			"на номер <code>{{.phone}}</code> в <b>{{.bank}}</b>\nс одним из комментариев:\n{{.comments}}\nи отправьте в чат фото платежа",
		varMonths, varPrice,
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Max:    maxComments * (64 + 4), // до 10 комментариев по 64 символа + «- » и перенос строки
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}).
		shown("По кнопке «Да» на подтверждении тарифа, когда платёж создан; кнопок нет, бот ждёт фото"),
	msg("payment.photo_sent", groupPayment, "Фото отправлено (без аванса)", photoSent).
		shown("После фото платежа, если лимит авансов исчерпан или выдать аванс не удалось", mainButtons...),
	msg("payment.photo_sent_advance", groupPayment, "Фото отправлено, выдан аванс",
		photoSent+"\n\nВам выдан доступ на {{.advance}} на время проверки",
		Var{Name: "advance", Description: "Длина аванса из настроек: «1 день»", Sample: "1 день", Max: 16}).
		shown("После фото платежа, когда аванс выдан", mainButtons...),
	msg("payment.confirmed", groupPayment, "Платёж подтверждён",
		"<b>Администратор подтвердил ваш платеж</b>\n\nВаша подписка была изменена").
		shown("Когда админ подтвердил платёж, через очередь уведомлений", mainButtons...),
	msg("payment.declined", groupPayment, "Платёж отклонён",
		"<b>Администратор отклонил ваш платеж</b>\n\nВаша подписка была изменена на изначальную").
		shown("Когда админ отклонил платёж, через очередь уведомлений", mainButtons...),

	msg("promo.enter_payment", groupPromo, "Ввод промокода при оплате", promoHead+"Отправьте промокод сообщением").
		shown("По кнопке «Есть промокод» на подтверждении тарифа", "button.back"),
	msg("promo.enter_menu", groupPromo, "Ввод промокода из меню", promoHead+"Отправьте промокод сообщением").
		shown("По кнопке «Промокод» в главном меню", "button.back"),
	msg("promo.not_found", groupPromo, "Промокод не найден", promoHead+"Такого промокода нет — проверьте написание и отправьте ещё раз").
		shown(reasonPromo+"такого кода нет", "button.back"),
	msg("promo.disabled", groupPromo, "Промокод выключен", promoHead+"Этот промокод больше не действует").
		shown(reasonPromo+"код выключен в админке", "button.back"),
	msg("promo.not_started", groupPromo, "Промокод ещё не действует", promoHead+"Этот промокод ещё не начал действовать").
		shown(reasonPromo+"срок действия кода ещё не наступил", "button.back"),
	msg("promo.expired", groupPromo, "Срок промокода истёк", promoHead+"Срок действия этого промокода истёк").
		shown(reasonPromo+"срок кода истёк", "button.back"),
	msg("promo.exhausted", groupPromo, "Активации закончились", promoHead+"Активации этого промокода закончились").
		shown(reasonPromo+"активации кода закончились", "button.back"),
	msg("promo.not_yours", groupPromo, "Персональный промокод другого юзера", promoHead+"Этот промокод предназначен другому пользователю").
		shown(reasonPromo+"код выдан другому юзеру", "button.back"),
	msg("promo.only_new", groupPromo, "Промокод только для новых", promoHead+"Этот промокод только для новых пользователей").
		shown(reasonPromo+"код только для новых, а у юзера есть оплата", "button.back"),
	msg("promo.already_used", groupPromo, "Промокод уже использован", promoHead+"Вы уже использовали этот промокод").
		shown(reasonPromo+"юзер уже активировал этот код", "button.back"),
	msg("promo.wrong_place_payment", groupPromo, "Код на дни введён при оплате",
		promoHead+"Этот промокод даёт дни — введите его через кнопку «Промокод» в меню").
		shown(reasonPromo+"при оплате прислан код на дни", "button.back"),
	msg("promo.wrong_place_menu", groupPromo, "Скидка введена из меню", promoHead+"Это скидка — введите её при оплате тарифа").
		shown(reasonPromo+"из меню прислан код на скидку", "button.back"),
	msg("promo.confirm_discount", groupPromo, "Подтверждение тарифа со скидкой",
		tariffHead+"Вы выбрали тариф на <b>{{.months}}</b> с промокодом <code>{{.code}}</code>\n\n"+
			"Цена: <s>{{.base}} ₽</s> <b>{{.price}} ₽</b>\n\nВерно?",
		varMonths,
		Var{Name: "base", Description: "Цена тарифа без скидки в рублях", Sample: "500", Max: 8},
		varPrice, varCode).
		shown("После присланного при оплате кода на скидку", "button.yes", "button.no", "button.remove_promo"),
	msg("promo.days_granted", groupPromo, "Дни по промокоду начислены",
		"<b>Промокод активирован</b>\n\nДобавлено дней: <b>{{.days}}</b>\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}",
		Var{Name: "days", Description: "Сколько дней дал промокод", Sample: "7", Max: 4}).
		shown("После присланного из меню кода на дни, когда срок продлён", mainButtons...),
	msg("promo.paid_by_promo", groupPromo, "Тариф оплачен промокодом (цена 0)",
		"<b>Подписка оплачена промокодом</b>\n\nТариф на <b>{{.months}}</b> активирован\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}",
		varMonths).
		shown("По «Да» на тарифе со скидкой 100%, через очередь уведомлений", mainButtons...),

	msg("reminder.week", groupReminder, "Подписка скоро закончится (первый порог)",
		reminderHead+"Ваша подписка скоро закончится"+reminderTail).
		shown("Раз в день, когда до конца подписки осталось меньше первого порога из настроек; один раз на дату окончания", "button.renew", "button.subscription"),
	msg("reminder.day", groupReminder, "Подписка заканчивается (второй порог)",
		reminderHead+"Ваша подписка закончится в течение суток"+reminderTail).
		shown("Раз в день, когда до конца подписки осталось меньше второго порога из настроек; один раз на дату окончания", "button.renew", "button.subscription"),

	msg("error.generic", groupError, "Ошибка", "К сожалению, произошла ошибка, обратитесь в поддержку").
		shown("Когда запрос юзера не удалось выполнить: сбой панели, базы или сохранения", "button.back", "button.support"),

	btn("button.info", "Информация", "Информация").on(routeInfo, StylePrimary, "Главное меню, если юзер есть в панели"),
	btn("button.pay", "Оплатить", "Оплатить").on(routePay, StyleDefault, "Главное меню, если юзер есть в панели"),
	btn("button.buy", "Купить", "Купить").on(routePay, StyleSuccess, "Главное меню и «Информация», если юзера нет в панели"),
	btn("button.renew", "Продлить", "Продлить").on(routePay, StyleDefault, "«Информация о подписке» и напоминания"),
	btn("button.subscription", "Подписка (копировать ссылку)", "Подписка").on("copy:ссылка подписки", StyleSuccess, "Главное меню, «Информация о подписке» и напоминания"),
	btn("button.support", "Поддержка", "Поддержка").on("url:supportUrl", StyleDanger, "Главное меню и сообщение об ошибке"),
	btn("button.back", "Назад", "Назад").on("menu | price | pay", StyleDefault, "Экраны ввода промокода, списка тарифов, «Информации» и ошибки"),
	btn("button.yes", "Да (подтвердить тариф)", "Да").on("buy | buyp", StyleSuccess, "Подтверждение тарифа, со скидкой и без"),
	btn("button.no", "Нет (к выбору тарифа)", "Нет").on(routePay, StyleDanger, "Подтверждение тарифа, со скидкой и без"),
	btn("button.tariff", "Тариф в списке", "{{.months}} за {{.price}} ₽", varMonths, varPrice).on("price", StyleDefault, "Список тарифов, по кнопке на тариф"),
	btn("button.service_down", "Сервис не работает", "Сервис не работает").on(routeMenu, StyleDefault, "Вместо главного меню, когда панель недоступна"),
	btn("button.promo", "Промокод (в меню)", "Промокод").on("promo", StyleDefault, "Главное меню"),
	btn("button.have_promo", "Есть промокод (при оплате)", "Есть промокод").on("have_promo", StyleDefault, "Подтверждение тарифа"),
	btn("button.remove_promo", "Убрать промокод", "Убрать промокод").on("price", StyleDefault, "Подтверждение тарифа со скидкой"),
}

// Globals — общие переменные (системные, затем свои по имени) с описанием и примером.
func Globals(custom map[string]string) []Var {
	out := slices.Clone(globals)
	for _, name := range slices.Sorted(maps.Keys(custom)) {
		// сохранённое до лимита длинное значение считаем по факту: худший случай не должен занижаться
		out = append(out, Var{Name: name, Description: "Своя переменная из настроек", Sample: custom[name], Max: max(MaxCustomVar, len(utf16.Encode([]rune(custom[name]))))})
	}
	return out
}

// Reserved — имя занято общей переменной или переменной какого-либо текста: своей так назвать нельзя.
func Reserved(name string) bool {
	has := func(v Var) bool { return v.Name == name }
	return slices.ContainsFunc(globals, has) ||
		slices.ContainsFunc(registry, func(d Def) bool { return slices.ContainsFunc(d.Vars, has) })
}

// Registry — все тексты по группам: Меню, Оплата, Промокоды, Напоминания, Ошибки, Кнопки.
func Registry() []Def {
	out := slices.Clone(registry)
	for i := range out {
		out[i].Vars = slices.Clone(out[i].Vars)
		out[i].Buttons = slices.Clone(out[i].Buttons)
	}
	return out
}

func Lookup(key string) (Def, bool) {
	i := slices.IndexFunc(registry, func(d Def) bool { return d.Key == key })
	if i < 0 {
		return Def{}, false
	}
	d := registry[i]
	d.Vars = slices.Clone(d.Vars)
	d.Buttons = slices.Clone(d.Buttons)
	return d, true
}
