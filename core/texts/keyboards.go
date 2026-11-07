package texts

import (
	"errors"
	"slices"

	"github.com/bruhabruh/bruhanet/core/usererr"
)

// Раскладка клавиатур: в коде — какие кнопки есть и что они делают, в админке — порядок, ряды (до двух кнопок) и
// удаление «за ненадобностью». Кнопка раскладки — ключ кнопки из реестра (подпись и стиль по-прежнему правятся
// через texts); динамический блок (`@…`) — кнопки, число которых решает код (тарифы, варианты устройств).

const (
	// MaxRowSlots — не больше кнопок в ряду.
	MaxRowSlots = 2
	// MaxRows — не больше рядов в раскладке.
	MaxRows = 8
)

// ErrBadLayout — раскладка не проходит ValidateLayout.
var ErrBadLayout = errors.New("invalid keyboard layout")

// Slot — кнопка раскладки. Removable: можно скрыть; нет — без неё сценарий упирается в тупик (да/нет, назад).
type Slot struct {
	ID        string
	Removable bool
}

// Block — динамический блок: двигается, но не скрывается, подпись и стиль у него общие с кнопкой `button.*`.
type Block struct{ ID, Title string }

// Keyboard — клавиатура под сообщениями Messages. Default — расположение, которое бот строил до появления раскладок.
type Keyboard struct {
	ID, Title string
	Messages  []string
	Slots     []Slot
	Blocks    []Block
	Default   [][]string
}

// Layout — сохранённая раскладка: ряды видимых кнопок и скрытые.
type Layout struct {
	Rows   [][]string `json:"rows"`
	Hidden []string   `json:"hidden"`
}

func (k Keyboard) slot(id string) (Slot, bool) {
	i := slices.IndexFunc(k.Slots, func(s Slot) bool { return s.ID == id })
	if i < 0 {
		return Slot{}, false
	}
	return k.Slots[i], true
}

func (k Keyboard) isBlock(id string) bool {
	return slices.ContainsFunc(k.Blocks, func(b Block) bool { return b.ID == id })
}

func (k Keyboard) declared(id string) bool {
	_, ok := k.slot(id)
	return ok || k.isBlock(id)
}

func bad(en, ru string, args ...any) error {
	return errors.Join(ErrBadLayout, usererr.Errorf(en, ru, args...))
}

// ValidateLayout проверяет раскладку админа: только объявленные кнопки, каждая не больше раза, до двух в ряду, до
// MaxRows рядов, динамический блок один в ряду, обязательные кнопки на месте, скрыты только те, что можно скрыть,
// пустых рядов нет (и после скрытия тоже).
func (k Keyboard) ValidateLayout(l Layout) error {
	if len(l.Rows) > MaxRows {
		return bad("more than %d rows", "рядов не больше %d", MaxRows)
	}
	hidden := map[string]bool{}
	for _, id := range l.Hidden {
		s, ok := k.slot(id)
		switch {
		case !ok:
			return bad("hidden slot %q is not hideable", "скрыть можно только кнопку, но не %q", id)
		case !s.Removable:
			return bad("slot %q is not removable", "кнопку %q скрывать нельзя", k.label(id))
		case hidden[id]:
			return bad("slot %q hidden twice", "кнопка %q скрыта дважды", k.label(id))
		}
		hidden[id] = true
	}
	seen := map[string]bool{}
	for _, row := range l.Rows {
		if len(row) == 0 {
			return bad("empty row", "пустой ряд")
		}
		if len(row) > MaxRowSlots {
			return bad("more than %d buttons in a row", "в ряду не больше %d кнопок", MaxRowSlots)
		}
		visible := 0
		for _, id := range row {
			switch {
			case !k.declared(id):
				return bad("unknown slot %q", "в этой клавиатуре нет кнопки %q", id)
			case seen[id]:
				return bad("slot %q is used twice", "кнопка %q стоит дважды", k.label(id))
			case k.isBlock(id) && len(row) > 1:
				return bad("block %q must be alone in a row", "блок %q должен стоять в ряду один", k.label(id))
			}
			seen[id] = true
			if !hidden[id] {
				visible++
			}
		}
		if visible == 0 {
			return bad("row left empty after hiding", "после скрытия кнопок ряд остался пустым")
		}
	}
	for _, s := range k.Slots {
		if !s.Removable && !seen[s.ID] {
			return bad("required slot %q is missing", "кнопку %q нельзя убрать", k.label(s.ID))
		}
	}
	for _, b := range k.Blocks {
		if !seen[b.ID] {
			return bad("block %q is missing", "блок %q нельзя убрать", b.Title)
		}
	}
	return nil
}

// label — подпись кнопки для сообщения админу.
func (k Keyboard) label(id string) string {
	for _, b := range k.Blocks {
		if b.ID == id {
			return b.Title
		}
	}
	if d, ok := Lookup(id); ok {
		return d.Title
	}
	return id
}

// Effective — раскладка, как её применяет бот: сохранённая (nil — по умолчанию), без скрытых; объявленные кнопки,
// которых в ней нет (появились в коде позже), встают отдельными рядами в порядке по умолчанию.
func (k Keyboard) Effective(stored *Layout) Layout {
	base := Layout{Rows: k.Default}
	if stored != nil {
		base = *stored
	}
	hidden := map[string]bool{}
	for _, id := range base.Hidden {
		hidden[id] = true
	}
	out := Layout{Rows: [][]string{}, Hidden: []string{}}
	seen := map[string]bool{}
	for _, row := range base.Rows {
		var r []string
		for _, id := range row {
			seen[id] = true
			if !hidden[id] {
				r = append(r, id)
			}
		}
		if len(r) > 0 {
			out.Rows = append(out.Rows, r)
		}
	}
	for _, id := range slices.Concat(k.Default...) {
		if !seen[id] && !hidden[id] {
			seen[id] = true
			out.Rows = append(out.Rows, []string{id})
		}
	}
	for _, id := range k.Order() {
		if hidden[id] {
			out.Hidden = append(out.Hidden, id)
		}
	}
	return out
}

// Order — кнопки и блоки в порядке по умолчанию, затем объявленные, но не стоящие в нём.
func (k Keyboard) Order() []string {
	ids := slices.Concat(k.Default...)
	for _, s := range k.Slots {
		if !slices.Contains(ids, s.ID) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// Arrange расставляет то, что построил код (по ключу кнопки или блока — ряды), по действующей раскладке: чего код не
// построил (условие не выполнено), того нет; блок раскрывается своими рядами на месте.
func Arrange[T any](k Keyboard, stored *Layout, built map[string][][]T) [][]T {
	var out [][]T
	for _, row := range k.Effective(stored).Rows {
		var cur []T
		for _, id := range row {
			rows, ok := built[id]
			if !ok {
				continue
			}
			if !k.isBlock(id) {
				cur = append(cur, slices.Concat(rows...)...)
				continue
			}
			if len(cur) > 0 {
				out, cur = append(out, cur), nil
			}
			out = append(out, rows...)
		}
		if len(cur) > 0 {
			out = append(out, cur)
		}
	}
	return out
}

const (
	blockTariffs = "@tariffs"
	blockDevices = "@devices"
)

func rm(ids ...string) []Slot { return flags(true, ids...) }
func fx(ids ...string) []Slot { return flags(false, ids...) }

func flags(removable bool, ids ...string) []Slot {
	out := make([]Slot, len(ids))
	for i, id := range ids {
		out[i] = Slot{ID: id, Removable: removable}
	}
	return out
}

var (
	blocks = map[string]Block{
		blockTariffs: {ID: blockTariffs, Title: "Тарифы"},
		blockDevices: {ID: blockDevices, Title: "Варианты числа устройств"},
	}
	yesNo = []string{"button.yes", "button.no"}
)

func kb(id, title string, messages []string, slots []Slot, blk []string, def ...[]string) Keyboard {
	k := Keyboard{ID: id, Title: title, Messages: messages, Slots: slots, Default: def}
	for _, b := range blk {
		k.Blocks = append(k.Blocks, blocks[b])
	}
	return k
}

func row(ids ...string) []string { return ids }

// keyboards — все клавиатуры, чью раскладку можно менять. Кнопки без выбора (одна «Назад» под сообщением,
// админские «Подтвердить/Отклонить», ссылка входа) сюда не входят: расставлять там нечего.
var keyboards = []Keyboard{
	kb("menu", "Главное меню", mainMenuMessages(), slices.Concat(
		rm("button.info", "button.pay", "button.subscription", "button.support", "button.trial", "button.promo", "button.devices", "button.gift", "button.referral")),
		nil,
		row("button.info", "button.pay"), row("button.subscription", "button.support"), row("button.trial"),
		row("button.promo", "button.devices"), row("button.gift"), row("button.referral")),
	kb("menu_new", "Главное меню, когда юзера нет в панели", mainMenuMessages(), slices.Concat(
		fx("button.buy"), rm("button.trial", "button.promo", "button.support", "button.gift", "button.referral")),
		nil,
		row("button.buy"), row("button.trial"), row("button.promo", "button.support"), row("button.gift"), row("button.referral")),
	kb("info", "Информация о подписке", []string{"menu.info"},
		slices.Concat(rm("button.renew", "button.subscription"), fx("button.back")), nil,
		row("button.renew", "button.subscription"), row("button.back")),
	kb("info_new", "Информация, подписка не куплена", []string{"menu.info_not_bought"},
		fx("button.buy", "button.back"), nil, row("button.buy"), row("button.back")),
	kb("tariffs", "Выбор тарифа", []string{"payment.tariffs"},
		fx("button.back"), []string{blockTariffs}, row(blockTariffs), row("button.back")),
	kb("gift_tariffs", "Подарок: выбор срока", []string{"gift.tariffs"},
		fx("button.back"), []string{blockTariffs}, row(blockTariffs), row("button.back")),
	kb("confirm", "Подтверждение тарифа", []string{"payment.confirm"},
		slices.Concat(fx(yesNo...), rm("button.have_promo")), nil, yesNo, row("button.have_promo")),
	kb("confirm_devices", "Подтверждение тарифа с доп. устройствами", []string{"payment.confirm_devices"},
		slices.Concat(fx(yesNo...), rm("button.drop_devices", "button.have_promo")), nil,
		yesNo, row("button.drop_devices"), row("button.have_promo")),
	kb("confirm_promo", "Подтверждение тарифа со скидкой", []string{"promo.confirm_discount"},
		fx("button.yes", "button.no", "button.remove_promo"), nil, yesNo, row("button.remove_promo")),
	kb("devices", "Устройства", []string{"devices.menu"},
		fx("button.devices_buy", "button.back"), nil, row("button.devices_buy"), row("button.back")),
	kb("devices_buy", "Докупка устройств", []string{"devices.buy"},
		fx("button.devices_custom", "button.back"), []string{blockDevices},
		row(blockDevices), row("button.devices_custom"), row("button.back")),
	kb("devices_no_term", "Устройства: подписка закончилась", []string{"devices.no_term"},
		slices.Concat(rm("button.renew"), fx("button.back")), nil, row("button.renew"), row("button.back")),
	kb("devices_confirm", "Подтверждение покупки устройств", []string{"devices.confirm"}, fx(yesNo...), nil, yesNo),
	kb("gift_confirm", "Подарок: подтверждение", []string{"gift.confirm"}, fx(yesNo...), nil, yesNo),
	kb("trial_confirm", "Пробный период: подтверждение", []string{"trial.confirm"}, fx(yesNo...), nil, yesNo),
	kb("referral", "Пригласить друга", []string{"referral.menu"},
		fx("button.copy_link", "button.back"), nil, row("button.copy_link"), row("button.back")),
	kb("gift_ready", "Подарок оплачен: ссылка", []string{"gift.ready"},
		fx("button.copy_link", "button.back"), nil, row("button.copy_link"), row("button.back")),
	kb("error", "Ошибка", []string{"error.generic"},
		slices.Concat(fx("button.back"), rm("button.support")), nil, row("button.back"), row("button.support")),
	kb("reminder", "Напоминания об окончании", []string{"reminder.week", "reminder.day"},
		rm("button.renew", "button.subscription"), nil, row("button.renew", "button.subscription")),
}

// mainMenuMessages — сообщения, под которыми бот показывает главное меню (те же кнопки, что у «Приветствия»).
func mainMenuMessages() []string {
	var out []string
	for _, d := range registry {
		if d.Kind == KindMessage && slices.Equal(d.Buttons, mainButtons) {
			out = append(out, d.Key)
		}
	}
	return out
}

// Keyboards — клавиатуры с настраиваемой раскладкой (копия).
func Keyboards() []Keyboard { return slices.Clone(keyboards) }

// LookupKeyboard — клавиатура по id.
func LookupKeyboard(id string) (Keyboard, bool) {
	i := slices.IndexFunc(keyboards, func(k Keyboard) bool { return k.ID == id })
	if i < 0 {
		return Keyboard{}, false
	}
	return keyboards[i], true
}

// KeyboardsFor — клавиатуры, которые бот показывает под сообщением key.
func KeyboardsFor(key string) []Keyboard {
	var out []Keyboard
	for _, k := range keyboards {
		if slices.Contains(k.Messages, key) {
			out = append(out, k)
		}
	}
	return out
}
