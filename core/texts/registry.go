// Package texts — реестр текстов бота, которые видит пользователь: значения по умолчанию,
// рендер {{.path}} в Telegram HTML и проверка правок.
package texts

import "slices"

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
}

const (
	groupMenu     = "Меню"
	groupPayment  = "Оплата"
	groupPromo    = "Промокоды"
	groupReminder = "Напоминания"
	groupError    = "Ошибки"
	groupButton   = "Кнопки"
)

var (
	varMonths   = Var{Name: "months", Description: "Срок тарифа с единицей: «3 месяца»", Sample: "3 месяца", Max: 16}
	varPrice    = Var{Name: "price", Description: "Цена тарифа в рублях", Sample: "500", Max: 8}
	varCode     = Var{Name: "code", Description: "Промокод", Sample: "SUMMER25", Max: 32}
	varExpireAt = Var{Name: "expireAt", Description: "Дата окончания подписки (МСК)", Sample: "2 ноября 2026 г. в 15:04, MSK", Max: 40}
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

var registry = []Def{
	msg("menu.greeting", groupMenu, "Приветствие", "Здравствуйте, <b>{{.name}}</b>",
		Var{Name: "name", Description: "Имя пользователя в Telegram", Sample: "Иван", Max: 64}),
	msg("menu.info", groupMenu, "Информация о подписке",
		infoHead+"{{.status}}\n\n<b>Трафик</b>\n{{.traffic}}\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}\n\n<b>Ваш Telegram ID</b>\n{{.telegramId}}",
		Var{Name: "status", Description: "Статус подписки (тексты «Статус: …»)", Sample: "Активна", Max: 64},
		Var{Name: "traffic", Description: "Использованный трафик из лимита", Sample: "1.50 ГБ из ∞", Max: 32},
		varExpireAt,
		Var{Name: "telegramId", Description: "Telegram ID пользователя", Sample: "123456789", Max: 20}),
	msg("menu.info_not_bought", groupMenu, "Информация: подписка не куплена", infoHead+"Не куплена"),
	plain("menu.status_active", "Статус: активна", "Активна"),
	plain("menu.status_disabled", "Статус: отключена", "Отключена"),
	plain("menu.status_limited", "Статус: ограничена", "Ограничена"),
	plain("menu.status_expired", "Статус: исчерпана", "Исчерпана"),
	plain("menu.status_unknown", "Статус: неизвестно", "Неизвестно"),

	msg("payment.tariffs", groupPayment, "Выбор тарифа", tariffHead+"Выберите тариф"),
	msg("payment.confirm", groupPayment, "Подтверждение тарифа",
		tariffHead+"Вы выбрали тариф на <b>{{.months}}</b> за <b>{{.price}} ₽</b>\n\nВерно?",
		varMonths, varPrice),
	msg("payment.requisites", groupPayment, "Реквизиты для оплаты",
		"<b>Оплата тарифа на {{.months}} за {{.price}} ₽</b>\n\n<b>Для оплаты</b>\nПереведите <b>{{.price}} ₽</b>\n"+
			"на номер <code>{{.phone}}</code> в <b>{{.bank}}</b>\nс одним из комментариев:\n{{.comments}}\nи отправьте в чат фото платежа",
		varMonths, varPrice,
		Var{Name: "phone", Description: "Номер для перевода (PAYMENT_PHONE)", Sample: "+79990000000", Max: 64},
		Var{Name: "bank", Description: "Банк для перевода (PAYMENT_BANK)", Sample: "Т-Банк", Max: 64},
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Max:    maxComments * (64 + 4), // до 10 комментариев по 64 символа + «- » и перенос строки
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}),
	msg("payment.photo_sent", groupPayment, "Фото отправлено (без аванса)", photoSent),
	msg("payment.photo_sent_advance", groupPayment, "Фото отправлено, выдан аванс",
		photoSent+"\n\nВам выдан доступ на {{.advance}} на время проверки",
		Var{Name: "advance", Description: "Длина аванса из настроек: «1 день»", Sample: "1 день", Max: 16}),
	msg("payment.confirmed", groupPayment, "Платёж подтверждён",
		"<b>Администратор подтвердил ваш платеж</b>\n\nВаша подписка была изменена"),
	msg("payment.declined", groupPayment, "Платёж отклонён",
		"<b>Администратор отклонил ваш платеж</b>\n\nВаша подписка была изменена на изначальную"),

	msg("promo.enter_payment", groupPromo, "Ввод промокода при оплате", promoHead+"Отправьте промокод сообщением"),
	msg("promo.enter_menu", groupPromo, "Ввод промокода из меню", promoHead+"Отправьте промокод сообщением"),
	msg("promo.not_found", groupPromo, "Промокод не найден", promoHead+"Такого промокода нет — проверьте написание и отправьте ещё раз"),
	msg("promo.disabled", groupPromo, "Промокод выключен", promoHead+"Этот промокод больше не действует"),
	msg("promo.expired", groupPromo, "Срок промокода истёк", promoHead+"Срок действия этого промокода истёк"),
	msg("promo.exhausted", groupPromo, "Активации закончились", promoHead+"Активации этого промокода закончились"),
	msg("promo.not_yours", groupPromo, "Персональный промокод другого юзера", promoHead+"Этот промокод предназначен другому пользователю"),
	msg("promo.only_new", groupPromo, "Промокод только для новых", promoHead+"Этот промокод только для новых пользователей"),
	msg("promo.already_used", groupPromo, "Промокод уже использован", promoHead+"Вы уже использовали этот промокод"),
	msg("promo.wrong_place_payment", groupPromo, "Код на дни введён при оплате",
		promoHead+"Этот промокод даёт дни — введите его через кнопку «Промокод» в меню"),
	msg("promo.wrong_place_menu", groupPromo, "Скидка введена из меню", promoHead+"Это скидка — введите её при оплате тарифа"),
	msg("promo.confirm_discount", groupPromo, "Подтверждение тарифа со скидкой",
		tariffHead+"Вы выбрали тариф на <b>{{.months}}</b> с промокодом <code>{{.code}}</code>\n\n"+
			"Цена: <s>{{.base}} ₽</s> <b>{{.price}} ₽</b>\n\nВерно?",
		varMonths,
		Var{Name: "base", Description: "Цена тарифа без скидки в рублях", Sample: "500", Max: 8},
		varPrice, varCode),
	msg("promo.days_granted", groupPromo, "Дни по промокоду начислены",
		"<b>Промокод активирован</b>\n\nДобавлено дней: <b>{{.days}}</b>\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}",
		Var{Name: "days", Description: "Сколько дней дал промокод", Sample: "7", Max: 4},
		varExpireAt),
	msg("promo.paid_by_promo", groupPromo, "Тариф оплачен промокодом (цена 0)",
		"<b>Подписка оплачена промокодом</b>\n\nТариф на <b>{{.months}}</b> активирован\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}",
		varMonths, varExpireAt),

	msg("reminder.week", groupReminder, "Подписка скоро закончится (первый порог)",
		reminderHead+"Ваша подписка скоро закончится"+reminderTail, varExpireAt),
	msg("reminder.day", groupReminder, "Подписка заканчивается (второй порог)",
		reminderHead+"Ваша подписка закончится в течение суток"+reminderTail, varExpireAt),

	msg("error.generic", groupError, "Ошибка", "К сожалению, произошла ошибка, обратитесь в поддержку"),

	btn("button.info", "Информация", "Информация"),
	btn("button.pay", "Оплатить", "Оплатить"),
	btn("button.buy", "Купить", "Купить"),
	btn("button.renew", "Продлить", "Продлить"),
	btn("button.subscription", "Подписка (копировать ссылку)", "Подписка"),
	btn("button.support", "Поддержка", "Поддержка"),
	btn("button.back", "Назад", "Назад"),
	btn("button.yes", "Да (подтвердить тариф)", "Да"),
	btn("button.no", "Нет (к выбору тарифа)", "Нет"),
	btn("button.tariff", "Тариф в списке", "{{.months}} за {{.price}} ₽", varMonths, varPrice),
	btn("button.service_down", "Сервис не работает", "Сервис не работает"),
	btn("button.promo", "Промокод (в меню)", "Промокод"),
	btn("button.have_promo", "Есть промокод (при оплате)", "Есть промокод"),
	btn("button.remove_promo", "Убрать промокод", "Убрать промокод"),
}

// Registry — все тексты по группам: Меню, Оплата, Промокоды, Напоминания, Ошибки, Кнопки.
func Registry() []Def {
	out := slices.Clone(registry)
	for i := range out {
		out[i].Vars = slices.Clone(out[i].Vars)
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
	return d, true
}
