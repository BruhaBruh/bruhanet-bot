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
type Var struct {
	Name, Description, Sample string
	Raw                       bool
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
	groupReminder = "Напоминания"
	groupError    = "Ошибки"
	groupButton   = "Кнопки"
)

var (
	varMonths   = Var{Name: "months", Description: "Срок тарифа с единицей: «3 месяца»", Sample: "3 месяца"}
	varPrice    = Var{Name: "price", Description: "Цена тарифа в рублях", Sample: "500"}
	varExpireAt = Var{Name: "expireAt", Description: "Дата окончания подписки (МСК)", Sample: "2 ноября 2026 г. в 15:04, MSK"}
)

const (
	photoSent    = "<b>Вы успешно отправили фото на проверку</b>"
	reminderHead = "<b>Уведомление об окончании подписки</b>\n\n"
	reminderTail = "\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}"
	tariffHead   = "<b>Выбор тарифа</b>\n\n"
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
		Var{Name: "name", Description: "Имя пользователя в Telegram", Sample: "Иван"}),
	msg("menu.info", groupMenu, "Информация о подписке",
		infoHead+"{{.status}}\n\n<b>Трафик</b>\n{{.traffic}}\n\n<b>Дата окончания подписки</b>\n{{.expireAt}}\n\n<b>Ваш Telegram ID</b>\n{{.telegramId}}",
		Var{Name: "status", Description: "Статус подписки (тексты «Статус: …»)", Sample: "Активна"},
		Var{Name: "traffic", Description: "Использованный трафик из лимита", Sample: "1.50 ГБ из ∞"},
		varExpireAt,
		Var{Name: "telegramId", Description: "Telegram ID пользователя", Sample: "123456789"}),
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
		Var{Name: "phone", Description: "Номер для перевода (PAYMENT_PHONE)", Sample: "+79990000000"},
		Var{Name: "bank", Description: "Банк для перевода (PAYMENT_BANK)", Sample: "Т-Банк"},
		Var{
			Name: "comments", Description: "Список комментариев к переводу, по строке «- <code>…</code>» на комментарий", Raw: true,
			Sample: "- <code>На кофе</code>\n- <code>Перекус</code>\n- <code>Одолжил</code>\n- <code>Вернул долг</code>",
		}),
	msg("payment.photo_sent", groupPayment, "Фото отправлено (без аванса)", photoSent),
	msg("payment.photo_sent_advance", groupPayment, "Фото отправлено, выдан 1 день",
		photoSent+"\n\nВам выдан доступ на 1 день на время проверки"),
	msg("payment.confirmed", groupPayment, "Платёж подтверждён",
		"<b>Администратор подтвердил ваш платеж</b>\n\nВаша подписка была изменена"),
	msg("payment.declined", groupPayment, "Платёж отклонён",
		"<b>Администратор отклонил ваш платеж</b>\n\nВаша подписка была изменена на изначальную"),

	msg("reminder.week", groupReminder, "Подписка скоро закончится (≤ 7 дней)",
		reminderHead+"Ваша подписка скоро закончится"+reminderTail, varExpireAt),
	msg("reminder.day", groupReminder, "Подписка закончится в течение суток",
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
}

// Registry — все тексты по группам: Меню, Оплата, Напоминания, Ошибки, Кнопки.
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
