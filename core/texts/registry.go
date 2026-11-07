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
	groupPayment  = "Оплата тарифа"
	groupDevices  = "Устройства"
	groupPromo    = "Промокоды"
	groupTrial    = "Пробный период"
	groupReferral = "Рефералка"
	groupGift     = "Подарки"
	groupReminder = "Уведомления"
	groupError    = "Ошибки"
	groupLogin    = "Вход"
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
	varMonths      = Var{Name: "months", Description: "Срок тарифа с единицей: «3 месяца»", Sample: "3 месяца", Max: 16}
	varMonthsShort = Var{Name: "monthsShort", Description: "Срок тарифа кратко: «3 мес.»", Sample: "3 мес.", Max: 10}
	varPrice       = Var{Name: "price", Description: "Цена тарифа в рублях", Sample: "500", Max: 8}
	varCode        = Var{Name: "code", Description: "Промокод", Sample: "SUMMER25", Max: 32}

	varTrialDays    = Var{Name: "days", Description: "Дней пробного периода", Sample: "3", Max: 4}
	varTrialDevices = Var{Name: "devices", Description: "Устройств на пробном периоде", Sample: "2", Max: 4}

	varReferralDays = Var{Name: "days", Description: "Дней каждому за первую оплату друга (из настроек)", Sample: "7", Max: 4}

	varGiftLink = Var{Name: "link", Description: "Ссылка на подарок: срабатывает один раз", Sample: "https://t.me/bot?start=gift_AbCdEfGhIjKlMnOpQrStUv", Max: 100}

	varDevicePrice   = Var{Name: "devicePrice", Description: "Цена одного доп. устройства в месяц, ₽", Sample: "50", Max: 8}
	varDevCountShort = Var{Name: "countShort", Description: "Сколько устройств добавляется, кратко: «2 устр.»", Sample: "2 устр.", Max: 12}
	varDevCount      = Var{Name: "count", Description: "Сколько устройств добавляется, с единицей: «2 устройства»", Sample: "2 устройства", Max: 16}
	varConnected     = Var{Name: "connected", Description: "Сколько устройств сейчас подключено (из панели); нет данных — «—»", Sample: "3", Max: 4}
	varDevMax        = Var{Name: "max", Description: "Наибольшее число устройств за одну докупку (из настроек)", Sample: "10", Max: 4}
	varDevTotal      = Var{Name: "price", Description: "Итоговая цена в рублях", Sample: "500", Max: 8}
)

// deviceVars — переменные экранов с лимитом устройств.
var deviceVars = []Var{
	{Name: "limit", Description: "Всего устройств в подписке", Sample: "7", Max: 4},
	{Name: "extra", Description: "Докупленных устройств", Sample: "2", Max: 4},
	varDevicePrice,
}

// maxComments — не больше комментариев к переводу (core/settings).
const maxComments = 10

const (
	photoSent    = "<b>📨 Фото получено</b>\n\nАдмин скоро проверит платёж."
	reminderHead = "<b>⏰ Подписка скоро закончится</b>\n\n"
	reminderTail = "\n\nДействует до: <b>{{.expireAt}}</b>"
	tariffHead   = "<b>💳 Выбор тарифа</b>\n\n"
	confirmHead  = "<b>🧾 Проверь заказ</b>\n\n"
	promoHead    = "<b>🏷 Промокод</b>\n\n"
	devicesHead  = "<b>📱 Устройства</b>\n\n"
	trialHead    = "<b>🎁 Пробный период</b>\n\n"
	referralHead = "<b>🤝 Пригласи друга</b>\n\n"
	giftHead     = "<b>🎁 Подарок</b>\n\n"
	infoHead     = "<b>📋 Информация о подписке</b>\n\n"

	// requisitesTail — как перевести деньги: общий хвост экранов оплаты тарифа, устройств и подарка.
	requisitesTail = "<b>Как оплатить</b>\n1. Переведи <b>{{.price}} ₽</b> на номер <code>{{.phone}}</code> в <b>{{.bank}}</b>\n" +
		"2. В комментарии укажи любой из вариантов:\n{{.comments}}\n3. Пришли сюда фото или скриншот перевода"
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
	"button.info", "button.pay", "button.buy", "button.subscription", "button.support", "button.promo", "button.devices", "button.trial", "button.referral", "button.gift", "button.service_down",
}

const reasonPromo = "В ответ на присланный промокод: "

var registry = []Def{
	msg("menu.greeting", groupMenu, "Приветствие", "Привет, <b>{{.name}}</b> 👋\n\nВыбери, что нужно, кнопками ниже.").
		shown("На /start, по кнопке «Назад» в меню и на любое сообщение вне сценария оплаты", mainButtons...),
	msg("menu.info", groupMenu, "Информация о подписке",
		infoHead+"<b>Статус:</b> {{.status}}\n<b>Трафик:</b> {{.traffic}}\n<b>Действует до:</b> {{.expireAt}}\n\n<b>Твой Telegram ID:</b> <code>{{.telegramId}}</code>",
		Var{Name: "status", Description: "Статус подписки (тексты «Статус: …»)", Sample: "Активна", Max: 64},
		Var{Name: "traffic", Description: "Использованный трафик из лимита", Sample: "1.50 ГБ из ∞", Max: 32},
	).shown("По кнопке «Информация», если юзер есть в панели", "button.renew", "button.subscription", "button.back"),
	msg("menu.info_not_bought", groupMenu, "Информация: подписка не куплена", infoHead+"Подписки пока нет. Нажми «Купить», чтобы оформить.").
		shown("По кнопке «Информация», если юзера нет в панели", "button.buy", "button.back"),
	plain("menu.status_active", "Статус: активна", "✅ Активна").shown("Подставляется в «Информацию о подписке», когда подписка активна"),
	plain("menu.status_disabled", "Статус: отключена", "⛔ Отключена").shown("Подставляется в «Информацию о подписке», когда юзер отключён в панели"),
	plain("menu.status_limited", "Статус: ограничена", "⚠️ Ограничена").shown("Подставляется в «Информацию о подписке», когда исчерпан лимит трафика"),
	plain("menu.status_expired", "Статус: исчерпана", "⌛ Закончилась").shown("Подставляется в «Информацию о подписке», когда срок истёк"),
	plain("menu.status_unknown", "Статус: неизвестно", "❔ Неизвестно").shown("Подставляется в «Информацию о подписке», когда панель вернула другой статус"),

	msg("payment.tariffs", groupPayment, "Выбор тарифа", tariffHead+"На сколько оформим подписку?").
		shown("По кнопкам «Оплатить», «Купить», «Продлить» и «Нет» на подтверждении тарифа", "button.tariff", "button.back"),
	msg("payment.confirm", groupPayment, "Подтверждение тарифа",
		confirmHead+"Тариф: <b>{{.months}}</b>\nК оплате: <b>{{.price}} ₽</b>\n\nВсё верно?",
		varMonths, varMonthsShort, varPrice).
		shown("После выбора тарифа в списке, а также по «Убрать промокод»", "button.yes", "button.no", "button.have_promo"),
	msg("payment.requisites", groupPayment, "Реквизиты для оплаты",
		"<b>💸 Оплата: {{.months}} за {{.price}} ₽</b>\n\n"+requisitesTail,
		varMonths, varMonthsShort, varPrice,
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Max:    maxComments * (64 + 4), // до 10 комментариев по 64 символа + «- » и перенос строки
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}).
		shown("По кнопке «Да» на подтверждении тарифа, когда платёж создан; кнопок нет, бот ждёт фото"),
	msg("payment.photo_sent", groupPayment, "Фото отправлено (без аванса)", photoSent).
		shown("После фото платежа, если лимит авансов исчерпан или выдать аванс не удалось", mainButtons...),
	msg("payment.photo_sent_advance", groupPayment, "Фото отправлено, выдан аванс",
		photoSent+"\n\n🎁 Пока идёт проверка, доступ открыт на {{.advance}}.",
		Var{Name: "advance", Description: "Длина аванса из настроек: «1 день»", Sample: "1 день", Max: 16}).
		shown("После фото платежа, когда аванс выдан", mainButtons...),
	msg("payment.confirmed", groupPayment, "Платёж подтверждён",
		"<b>✅ Платёж подтверждён</b>\n\nПодписка продлена, спасибо!\nДействует до: <b>{{.expireAt}}</b>").
		shown("Когда админ подтвердил платёж, через очередь уведомлений", mainButtons...),
	msg("payment.declined", groupPayment, "Платёж отклонён",
		"<b>❌ Платёж отклонён</b>\n\nСрок подписки вернулся к прежнему. Если это ошибка, напиши в поддержку.").
		shown("Когда админ отклонил платёж, через очередь уведомлений", mainButtons...),

	msg("payment.confirm_devices", groupDevices, "Подтверждение тарифа с доп. устройствами",
		confirmHead+"Тариф: <b>{{.months}}</b> — {{.tariff}} ₽\n"+
			"Доп. устройства: {{.devices}} × {{.devicePrice}} ₽ × {{.monthsCount}} = {{.addon}} ₽\n\n"+
			"Итого: <b>{{.price}} ₽</b>\n\nВсё верно?",
		varMonths, varMonthsShort,
		Var{Name: "tariff", Description: "Цена тарифа без устройств, ₽", Sample: "400", Max: 8},
		Var{Name: "devices", Description: "Сколько доп. устройств уже есть у юзера", Sample: "2", Max: 4},
		varDevicePrice,
		Var{Name: "monthsCount", Description: "Срок тарифа в месяцах числом", Sample: "3", Max: 4},
		Var{Name: "addon", Description: "Доплата за устройства за весь срок, ₽", Sample: "300", Max: 8},
		varDevTotal).
		shown("После выбора тарифа в списке, если у юзера есть доп. устройства (они входят в цену)",
			"button.yes", "button.no", "button.drop_devices", "button.have_promo"),
	msg("payment.devices_confirmed", groupDevices, "Устройства добавлены",
		"<b>✅ Платёж подтверждён</b>\n\nДобавлено устройств: <b>{{.devices}}</b>",
		Var{Name: "devices", Description: "Сколько устройств добавлено", Sample: "2", Max: 4}).
		shown("Когда админ подтвердил оплату устройств, через очередь уведомлений", mainButtons...),
	msg("payment.devices_declined", groupDevices, "Оплата устройств отклонена",
		"<b>❌ Платёж отклонён</b>\n\nКоличество устройств не изменилось.").
		shown("Когда админ отклонил оплату устройств, через очередь уведомлений", mainButtons...),
	msg("devices.menu", groupDevices, "Устройства",
		devicesHead+"Подключено: <b>{{.connected}}</b> из <b>{{.limit}}</b> (+{{.extra}})\n"+
			"Каждое дополнительное устройство: <b>{{.devicePrice}} ₽</b> в месяц",
		slices.Concat(deviceVars, []Var{varConnected})...).
		shown("По кнопке «Устройства» в главном меню, пока подписка действует", "button.devices_buy", "button.devices_clear", "button.devices_miniapp", "button.back"),
	msg("devices.clear_confirm", groupDevices, "Очистка устройств: подтверждение",
		devicesHead+"Отключить все подключённые устройства (сейчас <b>{{.connected}}</b>)? Потом их можно подключить заново, лимит подписки: <b>{{.limit}}</b>.\n\nВсё верно?",
		varConnected, Var{Name: "limit", Description: "Сколько устройств в подписке", Sample: "7", Max: 4}).
		shown("По кнопке «Очистить устройства» на экране «Устройства»", "button.yes", "button.no"),
	msg("devices.cleared", groupDevices, "Устройства очищены",
		"<b>✅ Устройства очищены</b>\n\nВсе устройства отключены от подписки. Подключай заново: можно до <b>{{.limit}}</b>.",
		Var{Name: "limit", Description: "Сколько устройств в подписке", Sample: "7", Max: 4}).
		shown("После подтверждения очистки устройств", "button.back"),
	msg("devices.buy", groupDevices, "Докупка устройств",
		devicesHead+"Выбери, сколько устройств добавить, или нажми «Своё число» (до {{.max}}).\n"+
			"Одно устройство — {{.devicePrice}} ₽ в месяц, цена считается до конца текущей подписки, срок не меняется.",
		varDevicePrice, varDevMax).
		shown("По кнопке «Докупить» на экране «Устройства»", "button.devices_add", "button.devices_custom", "button.back"),
	msg("devices.limit", groupDevices, "Докупка: предел устройств",
		devicesHead+"У тебя уже максимум устройств: <b>{{.limit}}</b>. Докупить больше нельзя.",
		Var{Name: "limit", Description: "Сколько устройств сейчас в подписке", Sample: "100", Max: 4}).
		shown("Когда сумма базовых и купленных устройств достигла предела панели (100)", "button.back"),
	msg("devices.custom", groupDevices, "Докупка: ввод своего числа",
		devicesHead+"Пришли число от 1 до <b>{{.max}}</b>: сколько устройств добавить.",
		varDevMax).
		shown("По кнопке «Своё число»: бот ждёт сообщение с числом", "button.back"),
	msg("devices.number_invalid", groupDevices, "Докупка: неверное число",
		devicesHead+"Нужно целое число от 1 до <b>{{.max}}</b>. Попробуй ещё раз.",
		varDevMax).
		shown("Когда в ответ на «Своё число» пришло не число или число вне допустимого", "button.back"),
	msg("devices.no_term", groupDevices, "Устройства: подписка закончилась",
		devicesHead+"Сейчас в подписке: <b>{{.limit}}</b> (+{{.extra}})\n"+
			"Каждое дополнительное устройство: <b>{{.devicePrice}} ₽</b> в месяц\n\n"+
			"Подписка закончилась, поэтому докупить устройства можно после продления. Уже купленные войдут в цену тарифа.",
		deviceVars...).
		shown("По кнопке «Устройства», когда подписка закончилась", "button.renew", "button.back"),
	msg("devices.confirm", groupDevices, "Подтверждение покупки устройств",
		devicesHead+"Добавить <b>{{.count}}</b> до конца подписки за <b>{{.price}} ₽</b>?\nСрок подписки не изменится.\n\nВсё верно?",
		varDevCount, varDevCountShort, varDevTotal).
		shown("После выбора количества устройств", "button.yes", "button.no"),
	msg("devices.requisites", groupDevices, "Реквизиты для оплаты устройств",
		"<b>💸 Оплата устройств: {{.count}}</b>\n\n"+requisitesTail,
		varDevCount, varDevCountShort, varDevTotal,
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Max:    maxComments * (64 + 4),
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}).
		shown("По «Да» на подтверждении покупки устройств, когда платёж создан; кнопок нет, бот ждёт фото"),
	msg("devices.busy", groupDevices, "Устройства: есть незавершённая оплата",
		devicesHead+"Сначала заверши текущую оплату: пришли фото платежа или нажми «Нет» на экране тарифа. Потом можно докупить устройства.").
		shown("Когда юзер хочет докупить устройства, а у него уже есть открытая оплата", "button.back"),

	msg("promo.enter_payment", groupPromo, "Ввод промокода при оплате", promoHead+"Пришли промокод сообщением.").
		shown("По кнопке «Есть промокод» на подтверждении тарифа", "button.back"),
	msg("promo.enter_menu", groupPromo, "Ввод промокода из меню", promoHead+"Пришли промокод сообщением.").
		shown("По кнопке «Промокод» в главном меню", "button.back"),
	msg("promo.not_found", groupPromo, "Промокод не найден", promoHead+"Такого промокода нет. Проверь, как он написан, и пришли ещё раз.").
		shown(reasonPromo+"такого кода нет", "button.back"),
	msg("promo.disabled", groupPromo, "Промокод выключен", promoHead+"Этот промокод больше не действует.").
		shown(reasonPromo+"код выключен в админке", "button.back"),
	msg("promo.not_started", groupPromo, "Промокод ещё не действует", promoHead+"Этот промокод ещё не начал действовать. Загляни позже.").
		shown(reasonPromo+"срок действия кода ещё не наступил", "button.back"),
	msg("promo.expired", groupPromo, "Срок промокода истёк", promoHead+"Срок действия этого промокода истёк.").
		shown(reasonPromo+"срок кода истёк", "button.back"),
	msg("promo.exhausted", groupPromo, "Активации закончились", promoHead+"Активации этого промокода закончились.").
		shown(reasonPromo+"активации кода закончились", "button.back"),
	msg("promo.not_yours", groupPromo, "Персональный промокод другого юзера", promoHead+"Этот промокод выдан другому пользователю.").
		shown(reasonPromo+"код выдан другому юзеру", "button.back"),
	msg("promo.only_new", groupPromo, "Промокод только для новых", promoHead+"Этот промокод только для новых пользователей.").
		shown(reasonPromo+"код только для новых, а у юзера есть оплата", "button.back"),
	msg("promo.already_used", groupPromo, "Промокод уже использован", promoHead+"Ты уже использовал этот промокод.").
		shown(reasonPromo+"юзер уже активировал этот код", "button.back"),
	msg("promo.wrong_place_payment", groupPromo, "Код на дни введён при оплате",
		promoHead+"Этот промокод даёт дни, а не скидку. Введи его через кнопку «Промокод» в меню.").
		shown(reasonPromo+"при оплате прислан код на дни", "button.back"),
	msg("promo.wrong_place_menu", groupPromo, "Скидка введена из меню", promoHead+"Это промокод на скидку. Введи его при оплате тарифа.").
		shown(reasonPromo+"из меню прислан код на скидку", "button.back"),
	msg("promo.confirm_discount", groupPromo, "Подтверждение тарифа со скидкой",
		confirmHead+"Тариф: <b>{{.months}}</b>\nПромокод: <code>{{.code}}</code>\n\n"+
			"К оплате: <s>{{.base}} ₽</s> <b>{{.price}} ₽</b>\n\nВсё верно?",
		varMonths, varMonthsShort,
		Var{Name: "base", Description: "Цена тарифа без скидки в рублях", Sample: "500", Max: 8},
		varPrice, varCode).
		shown("После присланного при оплате кода на скидку", "button.yes", "button.no", "button.remove_promo"),
	msg("promo.days_granted", groupPromo, "Дни по промокоду начислены",
		"<b>🎉 Промокод активирован</b>\n\nДобавлено дней: <b>{{.days}}</b>\nДействует до: <b>{{.expireAt}}</b>",
		Var{Name: "days", Description: "Сколько дней дал промокод", Sample: "7", Max: 4}).
		shown("После присланного из меню кода на дни, когда срок продлён", mainButtons...),
	msg("promo.paid_by_promo", groupPromo, "Тариф оплачен промокодом (цена 0)",
		"<b>🎉 Подписка оплачена промокодом</b>\n\nТариф на <b>{{.months}}</b> активирован\nДействует до: <b>{{.expireAt}}</b>",
		varMonths, varMonthsShort).
		shown("По «Да» на тарифе со скидкой 100%, через очередь уведомлений", mainButtons...),

	msg("trial.confirm", groupTrial, "Пробный период: подтверждение",
		trialHead+"Можно попробовать бесплатно: <b>{{.days}}</b> дн. и до <b>{{.devices}}</b> устройств. Пробный период доступен один раз.\n\nАктивировать?",
		varTrialDays, varTrialDevices).
		shown("По кнопке «Пробный период» в главном меню", "button.yes", "button.no"),
	msg("trial.started", groupTrial, "Пробный период выдан",
		"<b>🎁 Пробный период запущен</b>\n\nДней: <b>{{.days}}</b>\nУстройств: <b>{{.devices}}</b>\nДействует до: <b>{{.expireAt}}</b>",
		varTrialDays, varTrialDevices).
		shown("После выдачи пробного периода, через очередь уведомлений", mainButtons...),
	msg("trial.unavailable", groupTrial, "Пробный период недоступен", trialHead+"Сейчас пробный период недоступен.").
		shown("Когда юзер нажал «Пробный период», но получить его уже нельзя (причину не раскрываем)", "button.back"),

	msg("referral.menu", groupReferral, "Пригласить друга",
		referralHead+"Отправь другу свою ссылку. Когда он впервые оплатит подписку, вы оба получите по <b>{{.days}}</b> дн.\n\n"+
			"Твоя ссылка:\n<code>{{.link}}</code>",
		varReferralDays,
		Var{Name: "link", Description: "Личная ссылка юзера для приглашения", Sample: "https://t.me/bot?start=ref_123456789", Max: 100}).
		shown("По кнопке «Пригласить друга» в главном меню, пока дни рефералки в настройках больше нуля", "button.copy_link", "button.back"),
	msg("referral.friend_bonus", groupReferral, "Бонус другу за первую оплату",
		"<b>🎉 Бонус за первую оплату</b>\n\nТы пришёл по приглашению, поэтому мы добавили к подписке <b>{{.days}}</b> дн.\nДействует до: <b>{{.expireAt}}</b>",
		varReferralDays).
		shown("Отдельным сообщением после подтверждения первой оплаты приглашённого (не заблокированного админом), через очередь уведомлений", mainButtons...),
	msg("referral.referrer_bonus", groupReferral, "Бонус пригласившему",
		"<b>🎉 Друг оплатил подписку</b>\n\nТебе добавлено дней: <b>{{.days}}</b>\nДействует до: <b>{{.expireAt}}</b>",
		varReferralDays).
		shown("Пригласившему, когда его дни за первую оплату друга выданы (сразу или позже, если он был заблокирован), через очередь уведомлений", mainButtons...),

	msg("gift.tariffs", groupGift, "Подарок: выбор срока", giftHead+"На сколько подарим подписку?").
		shown("По кнопке «Подарить» в главном меню и «Нет» на подтверждении подарка", "button.tariff", "button.back"),
	msg("gift.confirm", groupGift, "Подарок: подтверждение",
		giftHead+"Подарить подписку на <b>{{.months}}</b> за <b>{{.price}} ₽</b>?\nТвой срок подписки не изменится: после оплаты ты получишь ссылку для друга, она сработает один раз.\n\nВсё верно?",
		varMonths, varMonthsShort, varPrice).
		shown("После выбора срока подарка", "button.yes", "button.no"),
	msg("gift.requisites", groupGift, "Подарок: реквизиты для оплаты",
		"<b>💸 Оплата подарка: {{.months}} за {{.price}} ₽</b>\n\n"+requisitesTail,
		varMonths, varMonthsShort, varPrice,
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Max:    maxComments * (64 + 4),
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}).
		shown("По «Да» на подтверждении подарка, когда платёж создан; кнопок нет, бот ждёт фото"),
	msg("gift.busy", groupGift, "Подарок: есть незавершённая оплата",
		giftHead+"Сначала заверши текущую оплату: пришли фото платежа или нажми «Нет» на экране тарифа. Потом можно оплатить подарок.").
		shown("Когда юзер хочет оплатить подарок, а у него уже есть открытая оплата", "button.back"),
	msg("gift.ready", groupGift, "Подарок оплачен: ссылка",
		"<b>✅ Подарок готов</b>\n\nОплата подтверждена, подписка на <b>{{.months}}</b> ждёт получателя. Отправь другу эту ссылку, она сработает один раз:\n<code>{{.link}}</code>\n\nТвой срок подписки не изменился.",
		varMonths, varMonthsShort, varGiftLink).
		shown("Когда админ подтвердил оплату подарка, через очередь уведомлений", "button.copy_link", "button.back"),
	msg("gift.declined", groupGift, "Оплата подарка отклонена",
		"<b>❌ Оплата отклонена</b>\n\nПодарок не создан, твоя подписка не изменилась.").
		shown("Когда админ отклонил оплату подарка, через очередь уведомлений", mainButtons...),
	msg("gift.cancelled", groupGift, "Подарок отозван (покупателю)",
		"<b>↩️ Подарок отозван</b>\n\nПодарок на <b>{{.months}}</b> администратор отозвал, ссылка больше не работает. Если нужна помощь или возврат, напиши в поддержку.",
		varMonths, varMonthsShort).
		shown("Покупателю, когда админ отозвал оплаченный подарок (подарок админа без покупателя ничего не шлёт), через очередь уведомлений", mainButtons...),
	msg("gift.redeemed", groupGift, "Подарок активирован (получателю)",
		"<b>🎁 Тебе подарили подписку</b>\n\nДобавлено: <b>{{.months}}</b>\nДействует до: <b>{{.expireAt}}</b>",
		varMonths, varMonthsShort).
		shown("Тому, кто открыл ссылку подарка и получил подписку, через очередь уведомлений", mainButtons...),
	msg("gift.redeemed_buyer", groupGift, "Подарок активирован (покупателю)",
		"<b>🎉 Подарок активирован</b>\n\nПодписку на <b>{{.months}}</b> получил тот, кому ты отправил ссылку.",
		varMonths, varMonthsShort).
		shown("Покупателю подарка, когда ссылкой воспользовались (кто именно, не сообщается; если покупатель сам, не отправляется)", mainButtons...),
	msg("gift.unavailable", groupGift, "Подарок недоступен", giftHead+"Этот подарок недоступен.").
		shown("Когда ссылка подарка не сработала (причину не раскрываем: нет такого, уже использован, отменён, нельзя юзеру)", "button.back"),

	msg("grant.days", groupReminder, "Админ добавил дни",
		"<b>🎁 Подписка продлена</b>\n\nАдмин добавил дней: <b>{{.days}}</b>\nДействует до: <b>{{.expireAt}}</b>",
		Var{Name: "days", Description: "Сколько дней добавил администратор", Sample: "7", Max: 4}).
		shown("Когда админ выдал дни из панели, через очередь уведомлений", mainButtons...),

	msg("reminder.week", groupReminder, "Подписка скоро закончится (первый порог)",
		reminderHead+"Продли заранее, чтобы не потерять доступ."+reminderTail).
		shown("Раз в день, когда до конца подписки осталось меньше первого порога из настроек; один раз на дату окончания", "button.renew", "button.subscription"),
	msg("reminder.day", groupReminder, "Подписка заканчивается (второй порог)",
		"<b>⏰ Подписка заканчивается</b>\n\nОна закончится в течение суток. Продли сейчас, чтобы доступ не прервался."+reminderTail).
		shown("Раз в день, когда до конца подписки осталось меньше второго порога из настроек; один раз на дату окончания", "button.renew", "button.subscription"),

	msg("login.message", groupLogin, "Вход в админ-панель", "Привет! Вы администратор {{.name}}. Нажмите кнопку ниже, чтобы войти в админ-панель.").
		shown("По /login от админа (и по ссылке входа со страницы панели)", "button.login"),
	msg("login.fallback", groupLogin, "Вход: ссылка вместо кнопки",
		"Привет! Вы администратор {{.name}}. Telegram не принял кнопку, поэтому ссылка здесь: нажми на неё, чтобы скопировать, и открой в браузере.\n\n<code>{{.link}}</code>",
		Var{Name: "link", Description: "Ссылка входа с одноразовым токеном", Sample: "http://localhost:5173/auth?token=AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-AbCdE", Max: 100}).
		shown("Вместо сообщения с кнопкой, если Telegram не принял адрес (например, localhost при разработке)"),

	msg("error.generic", groupError, "Ошибка", "<b>⚠️ Что-то пошло не так</b>\n\nНе получилось выполнить запрос. Попробуй ещё раз или напиши в поддержку.").
		shown("Когда запрос юзера не удалось выполнить: сбой панели, базы или сохранения", "button.back", "button.support"),

	btn("button.info", "Информация", "ℹ️ Информация").on(routeInfo, StylePrimary, "Главное меню, если юзер есть в панели"),
	btn("button.pay", "Оплатить", "💳 Оплатить").on(routePay, StyleDefault, "Главное меню, если юзер есть в панели"),
	btn("button.buy", "Купить", "🛒 Купить").on(routePay, StyleSuccess, "Главное меню и «Информация», если юзера нет в панели"),
	btn("button.renew", "Продлить", "🔄 Продлить").on(routePay, StyleDefault, "«Информация о подписке» и напоминания"),
	btn("button.subscription", "Подписка (копировать ссылку)", "🔗 Подписка").on("copy:ссылка подписки", StyleSuccess, "Главное меню, «Информация о подписке» и напоминания"),
	btn("button.support", "Поддержка", "💬 Поддержка").on("url:supportUrl", StyleDanger, "Главное меню и сообщение об ошибке"),
	btn("button.back", "Назад", "← Назад").on("menu | price | pay | devices | gift", StyleDefault, "Экраны ввода промокода, списка тарифов, «Информации», подарка и ошибки"),
	btn("button.yes", "Да (подтвердить тариф)", "✅ Да").on("buy | buyp | devy | trialy | gbuy", StyleSuccess, "Подтверждение тарифа, со скидкой и без, покупки устройств, подарка и пробного периода"),
	btn("button.no", "Нет (к выбору тарифа)", "✖️ Нет").on("pay | devices | devbuy | menu | gift", StyleDanger, "Подтверждение тарифа, со скидкой и без, покупки устройств, подарка и пробного периода"),
	btn("button.tariff", "Тариф в списке", "{{.months}} за {{.price}} ₽", varMonths, varMonthsShort, varPrice).on("price | gprice", StyleDefault, "Список тарифов и срок подарка, по кнопке на тариф"),
	btn("button.service_down", "Сервис не работает", "⚠️ Сервис не работает").on(routeMenu, StyleDefault, "Вместо главного меню, когда панель недоступна"),
	btn("button.promo", "Промокод (в меню)", "🏷 Промокод").on("promo", StyleDefault, "Главное меню"),
	btn("button.devices", "Устройства (в меню)", "📱 Устройства").on("devices", StyleDefault, "Главное меню, если юзер есть в панели и продажа устройств включена"),
	btn("button.devices_add", "Добавить устройства", "+{{.count}} — {{.price}} ₽", Var{Name: "count", Description: "Сколько устройств добавляется числом", Sample: "2", Max: 3}, varDevTotal).
		on("dev", StyleDefault, "Экран «Устройства», по кнопке на количество"),
	btn("button.devices_clear", "Очистить устройства", "🧹 Очистить устройства").on("devclr", StyleDefault, "Экран «Устройства»"),
	btn("button.devices_miniapp", "Управлять устройствами (MiniApp)", "📲 Управлять устройствами").on("url:miniAppUrl", StyleDefault, "Экран «Устройства», пока в настройках задана ссылка на MiniApp"),
	btn("button.devices_buy", "Докупить устройства", "➕ Докупить").on("devbuy", StyleDefault, "Экран «Устройства», пока подписка действует"),
	btn("button.devices_custom", "Своё число устройств", "✏️ Своё число").on("devc", StyleDefault, "Меню докупки устройств"),
	btn("button.drop_devices", "Без доп. устройств", "Без доп. устройств").on("buyd", StyleDefault, "Подтверждение тарифа, если у юзера есть доп. устройства"),
	btn("button.trial", "Пробный период (в меню)", "🎁 Пробный период").on("trial", StyleDefault, "Главное меню, пока пробный включён в настройках и юзер на него имеет право"),
	btn("button.referral", "Пригласить друга (в меню)", "🤝 Пригласить друга").on("referral", StyleDefault, "Главное меню, пока дни рефералки в настройках больше нуля"),
	btn("button.gift", "Подарить (в меню)", "🎁 Подарить").on("gift", StyleDefault, "Главное меню, пока в настройках есть тарифы"),
	btn("button.copy_link", "Скопировать ссылку приглашения или подарка", "📋 Скопировать ссылку").on("copy:ссылка приглашения или подарка", StyleSuccess, "Экран «Пригласить друга» и сообщение о готовом подарке"),
	btn("button.have_promo", "Есть промокод (при оплате)", "🏷 Есть промокод").on("have_promo", StyleDefault, "Подтверждение тарифа"),
	btn("button.remove_promo", "Убрать промокод", "✖️ Убрать промокод").on("price", StyleDefault, "Подтверждение тарифа со скидкой"),
	btn("button.login", "Войти в админ-панель", "Войти в админ-панель").on("url:ссылка входа", StylePrimary, "Сообщение со входом в админку, по /login от админа"),
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

// Registry — все тексты по группам: Меню, Оплата тарифа, Устройства, Промокоды, Пробный период, Рефералка, Подарки, Уведомления, Вход, Ошибки, Кнопки.
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
