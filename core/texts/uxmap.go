package texts

import "slices"

// UX-карта бота: откуда юзер попадает на экран и куда ведёт каждая кнопка. Таблицы вручную, потому что код бота
// ветвится (например, «Да» на оплате либо показывает реквизиты, либо «есть открытая оплата»); тест гарантирует, что
// каждый экран и каждая кнопка клавиатуры по умолчанию в них описаны.

// MapTarget — куда ведёт связь; Label — подпись стрелки, когда у связи нет кнопки.
type MapTarget struct {
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// MapEntry — вход в карту: команда, событие от админа/системы или ожидание ввода.
type MapEntry struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"` // command | event
	Title   string      `json:"title"`
	When    string      `json:"when"`
	Targets []MapTarget `json:"targets"`
}

// MapTransition — куда ведёт действие кнопки (Action как у Cell) на экране From; From "*" — для любого экрана.
type MapTransition struct {
	From    string   `json:"from"`
	Action  string   `json:"action"`
	Targets []string `json:"to"`
}

// MapLink — переход без кнопки: ввод юзера, решение админа, событие.
type MapLink struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// UXMap — таблицы карты одним ответом.
type UXMap struct {
	Entries     []MapEntry      `json:"entries"`
	Transitions []MapTransition `json:"transitions"`
	Links       []MapLink       `json:"links"`
}

func tg(to ...string) []MapTarget {
	out := make([]MapTarget, len(to))
	for i, t := range to {
		out[i] = MapTarget{To: t}
	}
	return out
}

var promoRejects = []string{
	"promo.not_found", "promo.disabled", "promo.not_started", "promo.expired", "promo.exhausted",
	"promo.not_yours", "promo.only_new", "promo.already_used",
}

var mapEntries = []MapEntry{
	{ID: "cmd_start", Kind: "command", Title: "/start", When: "Юзер запускает бота", Targets: tg("menu.greeting")},
	{ID: "cmd_any", Kind: "command", Title: "Любое сообщение", When: "Текст вне сценария оплаты", Targets: tg("menu.greeting")},
	{ID: "cmd_login", Kind: "command", Title: "/login", When: "Админ входит в панель (/start login от админа делает то же)", Targets: tg("login.message")},
	{ID: "cmd_gift", Kind: "command", Title: "/start gift_<код>", When: "Юзер открыл ссылку подарка", Targets: []MapTarget{
		{To: "gift.redeemed", Label: "подарок сработал"}, {To: "gift.unavailable", Label: "ссылка не подошла"},
	}},
	{ID: "cmd_photo", Kind: "command", Title: "Фото платежа", When: "Юзер прислал чек, пока есть открытая оплата", Targets: []MapTarget{
		{To: "payment.photo_sent", Label: "аванса нет"}, {To: "payment.photo_sent_advance", Label: "выдан аванс"},
	}},
	{ID: "ev_confirm", Kind: "event", Title: "Админ подтвердил платёж", When: "Кнопка в боте или в панели", Targets: []MapTarget{
		{To: "payment.confirmed", Label: "подписка"},
		{To: "payment.devices_confirmed", Label: "устройства"},
		{To: "gift.ready", Label: "подарок"},
		{To: "referral.friend_bonus", Label: "первая оплата по приглашению"},
	}},
	{ID: "ev_decline", Kind: "event", Title: "Админ отклонил платёж", When: "Кнопка в боте или в панели", Targets: []MapTarget{
		{To: "payment.declined", Label: "подписка"}, {To: "payment.devices_declined", Label: "устройства"}, {To: "gift.declined", Label: "подарок"},
	}},
	{ID: "ev_grant", Kind: "event", Title: "Админ выдал дни", When: "Кнопка «Выдать дни» в карточке юзера", Targets: tg("grant.days")},
	{ID: "ev_reminder", Kind: "event", Title: "Напоминание", When: "По расписанию, до конца подписки осталось мало", Targets: []MapTarget{
		{To: "reminder.week", Label: "первый порог"}, {To: "reminder.day", Label: "второй порог"},
	}},
	{ID: "ev_referrer", Kind: "event", Title: "Друг впервые оплатил", When: "Подтверждена первая оплата приглашённого", Targets: tg("referral.referrer_bonus")},
	{ID: "ev_gift_used", Kind: "event", Title: "Подарок активирован", When: "Кто-то открыл ссылку подарка", Targets: tg("gift.redeemed_buyer")},
	{ID: "ev_error", Kind: "event", Title: "Сбой", When: "Панель, база или сохранение не ответили", Targets: tg("error.generic")},
}

var mapTransitions = []MapTransition{
	// действия-маршруты одинаковы везде
	{"*", "route:menu", []string{"menu.greeting"}},
	{"*", "route:info", []string{"menu.info", "menu.info_not_bought"}},
	{"*", "route:pay", []string{"payment.tariffs"}},
	{"*", "route:promo", []string{"promo.enter_menu"}},
	{"*", "route:devices", []string{"devices.menu", "devices.no_term"}},
	{"*", "route:devices_buy", []string{"devices.buy"}},
	{"*", "route:trial", []string{"trial.confirm", "trial.unavailable"}},
	{"*", "route:gift", []string{"gift.tariffs"}},
	{"*", "route:referral", []string{"referral.menu"}},
	// шаблоны и «привязанные» действия зависят от экрана
	{"payment.tariffs", "template:tariffs", []string{"payment.confirm", "payment.confirm_devices"}},
	{"gift.tariffs", "template:gift_tariffs", []string{"gift.confirm"}},
	{"devices.buy", "template:devices", []string{"devices.confirm"}},
	{"devices.buy", "flow:devices_custom", []string{"devices.custom"}},
	{"payment.confirm", "flow:yes", []string{"payment.requisites"}},
	{"payment.confirm", "flow:no", []string{"payment.tariffs"}},
	{"payment.confirm", "flow:have_promo", []string{"promo.enter_payment"}},
	{"payment.confirm_devices", "flow:yes", []string{"payment.requisites"}},
	{"payment.confirm_devices", "flow:no", []string{"payment.tariffs"}},
	{"payment.confirm_devices", "flow:drop_devices", []string{"payment.requisites"}},
	{"payment.confirm_devices", "flow:have_promo", []string{"promo.enter_payment"}},
	{"promo.confirm_discount", "flow:yes", []string{"payment.requisites", "promo.paid_by_promo"}},
	{"promo.confirm_discount", "flow:no", []string{"payment.tariffs"}},
	{"promo.confirm_discount", "flow:remove_promo", []string{"payment.confirm"}},
	{"devices.confirm", "flow:yes", []string{"devices.requisites", "devices.busy"}},
	{"devices.confirm", "flow:no", []string{"devices.buy"}},
	{"gift.confirm", "flow:yes", []string{"gift.requisites", "gift.busy"}},
	{"gift.confirm", "flow:no", []string{"gift.tariffs"}},
	{"trial.confirm", "flow:yes", []string{"trial.started", "trial.unavailable"}},
	{"trial.confirm", "flow:no", []string{"menu.greeting"}},
	// «Назад» с исключениями
	{"devices.buy", "route:devices", []string{"devices.menu"}},
	// одиночная «Назад» (flow:back): куда вести, решает экран
	{"promo.enter_payment", "flow:back", []string{"payment.confirm", "payment.confirm_devices"}},
	{"promo.enter_menu", "flow:back", []string{"menu.greeting"}},
	{"promo.wrong_place_payment", "flow:back", []string{"payment.confirm", "payment.confirm_devices"}},
	{"promo.wrong_place_menu", "flow:back", []string{"menu.greeting"}},
	{"trial.unavailable", "flow:back", []string{"menu.greeting"}},
	{"devices.custom", "flow:back", []string{"devices.buy"}},
	{"devices.number_invalid", "flow:back", []string{"devices.buy"}},
	{"devices.busy", "flow:back", []string{"menu.greeting"}},
	{"devices.limit", "flow:back", []string{"devices.menu"}},
	{"devices.menu", "route:devices_buy", []string{"devices.buy", "devices.limit"}},
	{"gift.busy", "flow:back", []string{"menu.greeting"}},
	{"gift.unavailable", "flow:back", []string{"menu.greeting"}},
}

func init() {
	// отказы промокода показываются и при оплате, и из меню: «Назад» ведёт на экран, откуда юзер ввёл код
	for _, r := range promoRejects {
		mapTransitions = append(mapTransitions, MapTransition{r, "flow:back", []string{"menu.greeting", "payment.confirm", "payment.confirm_devices"}})
	}
}

// mapLinks — переходы без кнопки.
var mapLinks = func() []MapLink {
	base := []MapLink{
		{"payment.requisites", "payment.photo_sent", "юзер прислал чек, аванса нет"},
		{"payment.requisites", "payment.photo_sent_advance", "юзер прислал чек, выдан аванс"},
		{"devices.requisites", "payment.photo_sent", "юзер прислал чек"},
		{"gift.requisites", "payment.photo_sent", "юзер прислал чек"},
		{"payment.photo_sent", "payment.confirmed", "админ подтвердил"},
		{"payment.photo_sent", "payment.declined", "админ отклонил"},
		{"payment.photo_sent_advance", "payment.confirmed", "админ подтвердил"},
		{"payment.photo_sent_advance", "payment.declined", "админ отклонил"},
		{"devices.requisites", "payment.devices_confirmed", "админ подтвердил"},
		{"devices.requisites", "payment.devices_declined", "админ отклонил"},
		{"gift.requisites", "gift.ready", "админ подтвердил"},
		{"gift.requisites", "gift.declined", "админ отклонил"},
		{"promo.enter_payment", "promo.confirm_discount", "код подошёл"},
		{"promo.enter_payment", "promo.wrong_place_payment", "прислан код на дни"},
		{"promo.enter_menu", "promo.days_granted", "код подошёл"},
		{"promo.enter_menu", "promo.wrong_place_menu", "прислан код на скидку"},
		{"devices.custom", "devices.confirm", "число верное"},
		{"devices.custom", "devices.number_invalid", "не число или вне границ"},
		{"devices.number_invalid", "devices.confirm", "число верное"},
		{"referral.menu", "referral.friend_bonus", "друг впервые оплатил"},
		{"referral.menu", "referral.referrer_bonus", "друг впервые оплатил"},
		{"login.message", "login.fallback", "Telegram не принял кнопку"},
		{"gift.ready", "gift.redeemed_buyer", "ссылкой воспользовались"},
	}
	out := make([]MapLink, 0, len(base)+2*len(promoRejects))
	out = append(out, base...)
	for _, r := range promoRejects {
		out = append(out, MapLink{"promo.enter_payment", r, "код не подошёл"}, MapLink{"promo.enter_menu", r, "код не подошёл"})
	}
	return out
}()

// BuildUXMap — таблицы карты (копии).
func BuildUXMap() UXMap {
	m := UXMap{Entries: slices.Clone(mapEntries), Transitions: slices.Clone(mapTransitions), Links: slices.Clone(mapLinks)}
	for i := range m.Entries {
		m.Entries[i].Targets = slices.Clone(m.Entries[i].Targets)
	}
	for i := range m.Transitions {
		m.Transitions[i].Targets = slices.Clone(m.Transitions[i].Targets)
	}
	return m
}

// MapTargets — экраны, куда ведёт действие action с экрана from (нет записи — nil).
func MapTargets(from, action string) []string {
	for _, t := range mapTransitions {
		if t.Action == action && t.From == from {
			return t.Targets
		}
	}
	for _, t := range mapTransitions {
		if t.Action == action && t.From == "*" {
			return t.Targets
		}
	}
	return nil
}
