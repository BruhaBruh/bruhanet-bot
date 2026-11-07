package texts

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bruhabruh/bruhanet/core/usererr"
)

// Конструктор клавиатур: у каждого сообщения (ключ текста реестра) своя клавиатура из кнопок
// {id, подпись, стиль, действие, условия}. Код решает, что кнопка делает (действие из allow-list) и какие экраны
// клавиатуру вообще строят; админ — набор, порядок, подписи, стили, условия показа. Нет сохранённой раскладки —
// клавиатура по умолчанию (DefaultLayout), побайтно та, что бот строил до конструктора.

const (
	// MaxCells — не больше кнопок и шаблонов в клавиатуре сообщения.
	MaxCells = 16
	// ActionURL — префикс произвольной https-ссылки в действии.
	ActionURL = "url:"
	maxURLLen = 512
)

// Condition — условие показа кнопки: бот проверяет его при показе (bot/internal/handler/routes/conditions.go).
type Condition struct{ ID, Title, Description string }

var conditions = []Condition{
	{"in_panel", "Подписка заведена", "Юзер уже есть в панели (покупал, получал подарок или пробный период). Нет — юзер новый."},
	{"trial_available", "Пробный период доступен", "Пробный включён в настройках, юзер его ещё не брал, не платил и срока у него нет."},
	{"devices_sale_on", "Продажа устройств включена", "Цена дополнительного устройства в настройках больше нуля."},
	{"gifts_on", "Подарки доступны", "В настройках есть хотя бы один тариф."},
	{"referral_on", "Рефералка включена", "Дни за приглашённого друга в настройках больше нуля."},
}

// Conditions — каталог условий (копия).
func Conditions() []Condition { return slices.Clone(conditions) }

// LookupCondition — условие по id.
func LookupCondition(id string) (Condition, bool) {
	i := slices.IndexFunc(conditions, func(c Condition) bool { return c.ID == id })
	if i < 0 {
		return Condition{}, false
	}
	return conditions[i], true
}

// ActionKind — вид действия кнопки.
type ActionKind string

const (
	ActionRoute    ActionKind = "route"    // переход на экран бота
	ActionLink     ActionKind = "link"     // ссылка из настроек
	ActionURLKind  ActionKind = "url"      // https-ссылка админа (action = "url:<ссылка>", в каталоге не перечислена)
	ActionCopy     ActionKind = "copy"     // копирование значения, которое знает экран
	ActionFlow     ActionKind = "flow"     // служебное действие сценария: данные (месяц, тариф) знает код
	ActionTemplate ActionKind = "template" // блок кнопок, число которых решает код
)

// Action — действие из allow-list. Guard — встроенное ограничение: условие, без которого кнопка не показывается
// никогда (нельзя показать «Пробный», если он выключен). Bound: действие привязано к экрану — его можно ставить
// только туда, где оно есть в клавиатуре по умолчанию, и только на кнопку с тем же id.
type Action struct {
	ID, Title string
	Kind      ActionKind
	Guard     string
	Bound     bool
}

var actions = []Action{
	{ID: "route:menu", Title: "Главное меню", Kind: ActionRoute},
	{ID: "route:info", Title: "Информация о подписке", Kind: ActionRoute},
	{ID: "route:pay", Title: "Оплата и продление", Kind: ActionRoute},
	{ID: "route:promo", Title: "Промокод", Kind: ActionRoute},
	{ID: "route:devices", Title: "Устройства", Kind: ActionRoute, Guard: "devices_sale_on"},
	{ID: "route:devices_buy", Title: "Докупка устройств", Kind: ActionRoute, Guard: "devices_sale_on"},
	{ID: "route:trial", Title: "Пробный период", Kind: ActionRoute, Guard: "trial_available"},
	{ID: "route:gift", Title: "Подарить подписку", Kind: ActionRoute, Guard: "gifts_on"},
	{ID: "route:referral", Title: "Пригласить друга", Kind: ActionRoute, Guard: "referral_on"},
	{ID: "link:support", Title: "Поддержка (ссылка из настроек)", Kind: ActionLink},
	{ID: "link:login", Title: "Ссылка входа в админ-панель", Kind: ActionLink, Bound: true},
	{ID: "copy:subscription", Title: "Скопировать ссылку подписки", Kind: ActionCopy, Bound: true},
	{ID: "copy:referral_link", Title: "Скопировать реферальную ссылку", Kind: ActionCopy, Bound: true},
	{ID: "copy:gift_link", Title: "Скопировать ссылку подарка", Kind: ActionCopy, Bound: true},
	{ID: "flow:yes", Title: "Подтвердить", Kind: ActionFlow, Bound: true},
	{ID: "flow:no", Title: "Отказаться", Kind: ActionFlow, Bound: true},
	{ID: "flow:have_promo", Title: "Ввести промокод на тариф", Kind: ActionFlow, Bound: true},
	{ID: "flow:devices_custom", Title: "Ввести число устройств", Kind: ActionFlow, Bound: true},
	{ID: "flow:drop_devices", Title: "Тариф без дополнительных устройств", Kind: ActionFlow, Bound: true},
	{ID: "flow:remove_promo", Title: "Убрать промокод", Kind: ActionFlow, Bound: true},
	{ID: "template:tariffs", Title: "Тарифы", Kind: ActionTemplate, Bound: true},
	{ID: "template:gift_tariffs", Title: "Тарифы подарка", Kind: ActionTemplate, Bound: true},
	{ID: "template:devices", Title: "Варианты числа устройств", Kind: ActionTemplate, Bound: true},
}

// Actions — каталог действий (копия).
func Actions() []Action { return slices.Clone(actions) }

// LookupAction — действие каталога по id (https-ссылка админа в каталоге не числится).
func LookupAction(id string) (Action, bool) {
	i := slices.IndexFunc(actions, func(a Action) bool { return a.ID == id })
	if i < 0 {
		return Action{}, false
	}
	return actions[i], true
}

// Template — шаблон: блок кнопок, раскрываемый кодом (action "template:<id>").
// Key — текст кнопки реестра, которым подписана каждая кнопка блока (правится отдельно от клавиатуры).
type Template struct{ ID, Title, Key string }

var templateKeys = map[string]string{"tariffs": "button.tariff", "gift_tariffs": "button.tariff", "devices": "button.devices_add"}

// Templates — шаблоны, которые бот умеет раскрывать.
func Templates() []Template {
	var out []Template
	for _, a := range actions {
		if a.Kind == ActionTemplate {
			id := strings.TrimPrefix(a.ID, "template:")
			out = append(out, Template{ID: id, Title: a.Title, Key: templateKeys[id]})
		}
	}
	return out
}

// CondRef — условие на кнопке; Not переворачивает его. Условия кнопки соединяются через И.
type CondRef struct {
	ID  string `json:"id"`
	Not bool   `json:"not,omitempty"`
}

// Cell — кнопка (или шаблон) клавиатуры сообщения. Key: кнопка реестра, чья подпись и стиль действуют, пока свои не
// заданы (правка «Редактора сообщений» по-прежнему доходит до таких кнопок). Label — своя подпись с переменными;
// Style — свой стиль (пусто — как у Key, иначе как в коде).
type Cell struct {
	ID         string    `json:"id"`
	Key        string    `json:"key,omitempty"`
	Label      string    `json:"label,omitempty"`
	Style      string    `json:"style,omitempty"`
	Action     string    `json:"action"`
	Conditions []CondRef `json:"conditions"`
}

// MsgLayout — клавиатура сообщения: ряды кнопок.
type MsgLayout struct {
	Rows [][]Cell `json:"rows"`
}

// ErrBadMessageLayout — клавиатура сообщения не проходит проверку.
var ErrBadMessageLayout = errors.New("invalid message keyboard")

// badTitled — ошибка, где в русском тексте название (по-русски), а в английском — id.
func badTitled(en, ru, id, title string) error {
	return errors.Join(ErrBadMessageLayout, errors.Join(ErrBadLayout, &usererr.Error{En: fmt.Sprintf(en, id), Ru: fmt.Sprintf(ru, title)}))
}

func badMsg(en, ru string, args ...any) error {
	return errors.Join(ErrBadMessageLayout, bad(en, ru, args...))
}

// IsTemplate — ячейка шаблона.
func (c Cell) IsTemplate() bool { return strings.HasPrefix(c.Action, "template:") }

// Template — id шаблона ячейки ("" — это не шаблон).
func (c Cell) Template() string { return strings.TrimPrefix(c.Action, "template:") }

// cells — все ячейки по порядку.
func (l MsgLayout) cells() []Cell {
	var out []Cell
	for _, r := range l.Rows {
		out = append(out, r...)
	}
	return out
}

// Clone — глубокая копия (условия не делятся между копиями).
func (l MsgLayout) Clone() MsgLayout {
	out := MsgLayout{Rows: make([][]Cell, len(l.Rows))}
	for i, r := range l.Rows {
		out.Rows[i] = make([]Cell, len(r))
		for j, c := range r {
			c.Conditions = append([]CondRef{}, c.Conditions...)
			out.Rows[i][j] = c
		}
	}
	return out
}

// Equal — раскладки совпадают (nil и пустые условия — одно и то же).
func (l MsgLayout) Equal(o MsgLayout) bool {
	return slices.EqualFunc(l.Rows, o.Rows, func(a, b []Cell) bool {
		return slices.EqualFunc(a, b, func(x, y Cell) bool {
			return x.ID == y.ID && x.Key == y.Key && x.Label == y.Label && x.Style == y.Style && x.Action == y.Action &&
				slices.Equal(x.Conditions, y.Conditions)
		})
	})
}

// ---- клавиатуры по умолчанию ----

func notInPanel() []CondRef { return []CondRef{{ID: "in_panel", Not: true}} }
func inPanel() []CondRef    { return []CondRef{{ID: "in_panel"}} }

// cellSpec — кнопка слота slot клавиатуры kbID, как её строил код: действие и стиль на этом экране.
func cellSpec(kbID, slot string) Cell {
	c := Cell{ID: strings.TrimPrefix(slot, "button."), Key: slot, Conditions: []CondRef{}}
	switch slot {
	case "button.info":
		c.Action, c.Style = "route:info", StylePrimary
	case "button.pay":
		c.Action = "route:pay"
	case "button.buy":
		c.Action, c.Style = "route:pay", StyleSuccess
	case "button.renew":
		c.Action = "route:pay"
		if kbID == "devices_no_term" || kbID == "reminder" {
			c.Style = StyleSuccess
		}
	case "button.subscription":
		c.Action, c.Style = "copy:subscription", StyleSuccess
		if kbID == "reminder" {
			c.Style = StylePrimary
		}
	case "button.support":
		c.Action, c.Style = "link:support", StyleDanger
		if kbID == "error" {
			c.Style = StyleDefault
		}
	case "button.promo":
		c.Action = "route:promo"
	case "button.trial":
		c.Action = "route:trial"
	case "button.devices":
		c.Action = "route:devices"
	case "button.login":
		c.Action, c.Style = "link:login", StylePrimary
	case "button.devices_buy":
		c.Action = "route:devices_buy"
	case "button.devices_custom":
		c.Action = "flow:devices_custom"
	case "button.gift":
		c.Action = "route:gift"
	case "button.referral":
		c.Action = "route:referral"
	case "button.back":
		c.Action = "route:menu"
		if kbID == "devices_buy" { // «Назад» из меню докупки ведёт на экран «Устройства»
			c.Action = "route:devices"
		}
	case "button.yes":
		c.Action, c.Style = "flow:yes", StyleSuccess
	case "button.no":
		c.Action, c.Style = "flow:no", StyleDanger
	case "button.have_promo":
		c.Action = "flow:have_promo"
	case "button.drop_devices":
		c.Action = "flow:drop_devices"
	case "button.remove_promo":
		c.Action = "flow:remove_promo"
	case "button.copy_link":
		c.Action, c.Style = "copy:referral_link", StyleSuccess
		if kbID == "gift_ready" {
			c.Action = "copy:gift_link"
		}
	}
	return c
}

// templateCell — ячейка шаблона вместо динамического блока клавиатуры.
func templateCell(kbID, block string) Cell {
	name := "tariffs"
	switch {
	case block == blockDevices:
		name = "devices"
	case kbID == "gift_tariffs":
		name = "gift_tariffs"
	}
	return Cell{ID: "tpl_" + name, Action: "template:" + name, Conditions: []CondRef{}}
}

func keyboardCells(k Keyboard, rows [][]string) [][]Cell {
	out := make([][]Cell, 0, len(rows))
	for _, r := range rows {
		cur := make([]Cell, 0, len(r))
		for _, id := range r {
			if k.isBlock(id) {
				cur = append(cur, templateCell(k.ID, id))
			} else {
				cur = append(cur, cellSpec(k.ID, id))
			}
		}
		out = append(out, cur)
	}
	return out
}

func isMenuPair(ks []Keyboard) bool {
	return len(ks) == 2 && ks[0].ID == "menu" && ks[1].ID == "menu_new"
}

// menuDefault — «menu» и «menu_new» одной клавиатурой: общие ряды, а кнопки, различающиеся по юзеру, разведены
// условием «Подписка заведена». Отфильтрованная по нему, она совпадает с каждой из двух.
func menuDefault() ([]Keyboard, MsgLayout) {
	ks := []Keyboard{keyboardByID("menu"), keyboardByID("menu_new")}
	in := func(c Cell) Cell { c.Conditions = inPanel(); return c }
	out := func(c Cell) Cell { c.Conditions = notInPanel(); return c }
	renamed := func(c Cell, id string) Cell { c.ID = id; return c }
	return ks, MsgLayout{Rows: [][]Cell{
		{out(cellSpec("menu_new", "button.buy"))},
		{in(cellSpec("menu", "button.info")), in(cellSpec("menu", "button.pay"))},
		{in(cellSpec("menu", "button.subscription")), in(cellSpec("menu", "button.support"))},
		{cellSpec("menu", "button.trial")},
		{out(renamed(cellSpec("menu_new", "button.promo"), "promo_new")), out(renamed(cellSpec("menu_new", "button.support"), "support_new"))},
		{in(cellSpec("menu", "button.promo")), in(cellSpec("menu", "button.devices"))},
		{cellSpec("menu", "button.gift")},
		{cellSpec("menu", "button.referral")},
	}}
}

func keyboardByID(id string) Keyboard {
	k, _ := LookupKeyboard(id)
	return k
}

// MessageDefault — клавиатура по умолчанию сообщения key и id нельзя убрать (служебные кнопки и шаблоны); ok=false:
// под сообщением настраиваемой клавиатуры нет.
func MessageDefault(key string) (MsgLayout, []string, bool) {
	ks := KeyboardsFor(key)
	switch {
	case len(ks) == 0:
		return MsgLayout{}, nil, false
	case isMenuPair(ks):
		_, l := menuDefault()
		return l, []string{"buy"}, true
	}
	k := ks[0]
	var locked []string
	for _, id := range k.Order() {
		if s, ok := k.slot(id); ok && !s.Removable {
			locked = append(locked, strings.TrimPrefix(id, "button."))
		}
	}
	l := MsgLayout{Rows: keyboardCells(k, k.Default)}
	for _, c := range l.cells() {
		if c.IsTemplate() {
			locked = append(locked, c.ID)
		}
	}
	return l, locked, true
}

// MessagesWithKeyboard — сообщения, под которыми есть настраиваемая клавиатура (порядок реестра).
func MessagesWithKeyboard() []string {
	var out []string
	for _, d := range registry {
		if _, _, ok := MessageDefault(d.Key); ok {
			out = append(out, d.Key)
		}
	}
	return out
}

// EffectiveLayout — клавиатура сообщения, как её применяет бот: сохранённая (nil — по умолчанию).
func EffectiveLayout(key string, stored *MsgLayout) (MsgLayout, bool) {
	def, _, ok := MessageDefault(key)
	if !ok {
		return MsgLayout{}, false
	}
	if stored != nil {
		return stored.Clone(), true
	}
	return def, true
}

// LegacyToLayout переводит сохранённые раскладки общих клавиатур (text_keyboards) в клавиатуру сообщения key:
// ok=false — у сообщения не правили ни одной из его клавиатур. Главное меню двух клавиатур собирается из рядов обеих
// (меню юзера в панели, затем меню нового юзера), развёрнутых условием «Подписка заведена».
func LegacyToLayout(key string, legacy map[string]Layout) (MsgLayout, bool) {
	ks := KeyboardsFor(key)
	touched := false
	for _, k := range ks {
		_, has := legacy[k.ID]
		touched = touched || has
	}
	if !touched {
		return MsgLayout{}, false
	}
	rowsOf := func(k Keyboard) [][]Cell {
		l, has := legacy[k.ID]
		if !has {
			return keyboardCells(k, k.Default)
		}
		return keyboardCells(k, k.Effective(&l).Rows)
	}
	if !isMenuPair(ks) {
		return MsgLayout{Rows: rowsOf(ks[0])}, true
	}
	var out MsgLayout
	for i, k := range ks {
		for _, row := range rowsOf(k) {
			r := make([]Cell, len(row))
			for j, c := range row {
				if i == 1 && c.ID != "buy" {
					c.ID += "_new"
				}
				c.Conditions = inPanel()
				if i == 1 {
					c.Conditions = notInPanel()
				}
				r[j] = c
			}
			out.Rows = append(out.Rows, r)
		}
	}
	return out, true
}

// ---- проверка ----

var customID = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// ValidateMessageLayout — единственная проверка клавиатуры сообщения (API при сохранении). custom — свои переменные
// настроек для проверки подписи. strict=false (бот при загрузке) не смотрит на лимиты рядов и кнопок, чтобы не
// выбрасывать раскладки, перенесённые из старых общих клавиатур.
func ValidateMessageLayout(key string, l MsgLayout, custom map[string]string, strict bool) error {
	def, locked, ok := MessageDefault(key)
	if !ok {
		return badMsg("message %q has no keyboard", "у сообщения %q нет клавиатуры", key)
	}
	cells := l.cells()
	if strict {
		if len(l.Rows) > MaxRows {
			return badMsg("more than %d rows", "рядов не больше %d", MaxRows)
		}
		if len(cells) > MaxCells {
			return badMsg("more than %d buttons", "кнопок не больше %d", MaxCells)
		}
	}
	defByID := map[string]Cell{}
	defByAction := map[string]Cell{}
	for _, c := range def.cells() {
		defByID[c.ID] = c
		defByAction[c.Action] = c
	}
	for _, row := range l.Rows {
		if len(row) == 0 {
			return badMsg("empty row", "пустой ряд")
		}
		if len(row) > MaxRowSlots {
			return badMsg("more than %d buttons in a row", "в ряду не больше %d кнопок", MaxRowSlots)
		}
		for _, c := range row {
			if c.IsTemplate() && len(row) > 1 {
				return badMsg("template %q must be alone in a row", "шаблон «%s» должен стоять в ряду один", c.ID)
			}
		}
	}
	ids := map[string]bool{}
	bound := map[string]bool{}
	for _, c := range cells {
		if err := validateCell(c, defByID, defByAction, custom); err != nil {
			return err
		}
		if ids[c.ID] {
			return badMsg("id %q is used twice", "id %q повторяется", c.ID)
		}
		ids[c.ID] = true
		if a, _ := LookupAction(c.Action); a.Bound {
			if bound[c.Action] {
				return badTitled("action %q is used twice", "действие «%s» уже стоит на другой кнопке", c.Action, a.Title)
			}
			bound[c.Action] = true
		}
	}
	for _, id := range locked {
		c, present := cellByID(cells, id)
		d := defByID[id]
		switch {
		case !present:
			return badTitled("required button %q is missing", "кнопку «%s» нельзя убрать", id, cellTitle(d))
		case c.Action != d.Action || !slices.Equal(c.Conditions, d.Conditions):
			return badTitled("required button %q changed action or conditions", "у кнопки «%s» нельзя менять действие и условия", id, cellTitle(d))
		}
	}
	return nil
}

func cellByID(cells []Cell, id string) (Cell, bool) {
	i := slices.IndexFunc(cells, func(c Cell) bool { return c.ID == id })
	if i < 0 {
		return Cell{}, false
	}
	return cells[i], true
}

// cellTitle — название кнопки для сообщения админу.
func cellTitle(c Cell) string {
	if d, ok := Lookup(c.Key); ok && c.Key != "" {
		return d.Title
	}
	if a, ok := LookupAction(c.Action); ok {
		return a.Title
	}
	return c.ID
}

func validateCell(c Cell, defByID, defByAction map[string]Cell, custom map[string]string) error {
	_, system := defByID[c.ID]
	if !system && !customID.MatchString(c.ID) {
		return badMsg("invalid id %q", "id %q: латиница в нижнем регистре, цифры и _, до 32 символов", c.ID)
	}
	if c.IsTemplate() {
		if a, ok := LookupAction(c.Action); !ok || a.Kind != ActionTemplate {
			return badMsg("unknown template %q", "неизвестный шаблон %q", c.Action)
		}
		d, ok := defByAction[c.Action]
		if !ok || d.ID != c.ID {
			return badMsg("template %q is not available here", "шаблона %q у этого сообщения нет", c.Action)
		}
		if c.Label != "" || c.Style != "" || c.Key != "" || len(c.Conditions) > 0 {
			return badMsg("template %q takes no label, style or conditions", "у шаблона %q нет подписи, стиля и условий", c.ID)
		}
		return nil
	}
	guard, err := validateAction(c, defByAction)
	if err != nil {
		return err
	}
	if c.Key != "" {
		if d, ok := Lookup(c.Key); !ok || d.Kind != KindButton {
			return badMsg("unknown button text %q", "неизвестный текст кнопки %q", c.Key)
		}
	}
	switch {
	case c.Label == "" && c.Key == "":
		return badMsg("button %q has no label", "у кнопки %q нет подписи", c.ID)
	case c.Label != "":
		if err := Validate(Def{Key: c.ID, Kind: KindButton, Title: "Кнопка"}, c.Label, custom); err != nil {
			return errors.Join(ErrBadMessageLayout, err)
		}
	}
	if !slices.Contains(Styles, c.Style) {
		return badMsg("unknown style %q", "неизвестный стиль %q", c.Style)
	}
	seen := map[string]bool{}
	for _, cr := range c.Conditions {
		if _, ok := LookupCondition(cr.ID); !ok {
			return badMsg("unknown condition %q", "неизвестное условие %q", cr.ID)
		}
		if seen[cr.ID] {
			return badTitled("condition %q is listed twice", "условие «%s» указано дважды", cr.ID, conditionTitle(cr.ID))
		}
		seen[cr.ID] = true
		if guard != "" && cr.ID == guard && cr.Not {
			return badTitled("condition %q contradicts the action guard", "кнопка не может показаться никогда: условие «НЕ %s» противоречит встроенному ограничению действия", cr.ID, conditionTitle(cr.ID))
		}
	}
	return nil
}

func conditionTitle(id string) string {
	c, _ := LookupCondition(id)
	return c.Title
}

// validateAction проверяет действие кнопки; возвращает встроенное условие действия.
func validateAction(c Cell, defByAction map[string]Cell) (string, error) {
	if raw, ok := strings.CutPrefix(c.Action, ActionURL); ok {
		if !ValidHTTPSURL(raw, maxURLLen) {
			return "", badMsg("url must be https and at most %d characters", "ссылка должна начинаться с https:// и быть не длиннее %d символов", maxURLLen)
		}
		return "", nil
	}
	a, ok := LookupAction(c.Action)
	if !ok {
		return "", badMsg("unknown action %q", "неизвестное действие %q", c.Action)
	}
	if a.Bound {
		if d, has := defByAction[c.Action]; !has || d.ID != c.ID {
			return "", badTitled("action %q is not available here", "действие «%s» здесь недоступно", c.Action, a.Title)
		}
	}
	return a.Guard, nil
}
