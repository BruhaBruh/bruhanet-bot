package routes

import (
	"context"
	neturl "net/url"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// Кнопки с подписью и стилем из реестра: style — вид на этом экране, пока админ не сохранил свой.

func callbackButton(ctx context.Context, c *content.Provider, key, data string, style keyboard.Style) keyboard.Button {
	return keyboard.Callback(c.Button(ctx, key), data).Style(c.Style(ctx, key, style))
}

func urlButton(ctx context.Context, c *content.Provider, key, url string, style keyboard.Style) keyboard.Button {
	return keyboard.URL(c.Button(ctx, key), url).Style(c.Style(ctx, key, style))
}

func webAppButton(ctx context.Context, c *content.Provider, key, url string, style keyboard.Style) keyboard.Button {
	return keyboard.WebApp(c.Button(ctx, key), url).Style(c.Style(ctx, key, style))
}

// miniAppURL — адрес Mini App (MINIAPP_ORIGIN); tab открывает вкладку (пусто — главная).
func miniAppURL(base, tab string) string {
	if tab == "" {
		return base
	}
	u, err := neturl.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("tab", tab)
	u.RawQuery = q.Encode()
	return u.String()
}

func copyButton(ctx context.Context, c *content.Provider, key, value string, style keyboard.Style) keyboard.Button {
	return keyboard.Copy(c.Button(ctx, key), value).Style(c.Style(ctx, key, style))
}

// kbBuilder собирает клавиатуру сообщения key. Код создаёт кнопки по своим условиям (callback/url/copy/block), а
// вид решает раскладка: клавиатура сообщения из конструктора (texts.MsgLayout: свои подписи, стили, действия и
// условия), иначе — по умолчанию из кода (общая раскладка клавиатуры id, texts.Keyboard, как до конструктора).
type kbBuilder struct {
	ctx   context.Context
	c     *content.Provider
	id    string // клавиатура по умолчанию (texts.Keyboard)
	key   string // сообщение, под которым она стоит
	built map[string][][]keyboard.Button
	args  map[string]string // ключ кнопки → callback data / значение для copy, как их передал экран
	env   condEnv
}

func newKB(ctx context.Context, c *content.Provider, id, key string) *kbBuilder {
	return &kbBuilder{
		ctx: ctx, c: c, id: id, key: key, built: map[string][][]keyboard.Button{}, args: map[string]string{},
		env: condEnv{ctx: ctx, c: c},
	}
}

func (b *kbBuilder) put(key string, btn keyboard.Button) *kbBuilder {
	b.built[key] = [][]keyboard.Button{{btn}}
	return b
}

func (b *kbBuilder) callback(key, data string, style keyboard.Style) *kbBuilder {
	b.args[key] = data
	return b.put(key, callbackButton(b.ctx, b.c, key, data, style))
}

func (b *kbBuilder) url(key, url string, style keyboard.Style) *kbBuilder {
	b.args[key] = url
	return b.put(key, urlButton(b.ctx, b.c, key, url, style))
}

// webApp — кнопка Mini App; url уже с вкладкой (miniAppURL).
func (b *kbBuilder) webApp(key, url string, style keyboard.Style) {
	b.args[key] = url
	b.put(key, webAppButton(b.ctx, b.c, key, url, style))
}

func (b *kbBuilder) copy(key, value string, style keyboard.Style) *kbBuilder {
	b.args[key] = value
	return b.put(key, copyButton(b.ctx, b.c, key, value, style))
}

// block кладёт динамический блок (`@…`): его ряды остаются рядами.
func (b *kbBuilder) block(id string, rows [][]keyboard.Button) *kbBuilder {
	b.built[id] = rows
	return b
}

// markup — разметка по действующей раскладке; все кнопки скрыты — nil (сообщение без клавиатуры).
func (b *kbBuilder) markup() *models.InlineKeyboardMarkup {
	if lay := b.c.MessageLayout(b.ctx, b.key); lay != nil {
		return b.fromLayout(*lay)
	}
	if b.id == "" { // у сообщения нет кнопок по умолчанию: без сохранённых админом клавиатуры нет
		return nil
	}
	k, ok := texts.LookupKeyboard(b.id)
	if !ok {
		stlog.Error(b.ctx, "unknown keyboard", "keyboard", b.id)
		return nil
	}
	rows := texts.Arrange(k, b.c.Layout(b.ctx, b.id), b.built)
	if len(rows) == 0 {
		return nil
	}
	return keyboard.Inline(rows...)
}

// routeData — действия `route:<имя>` конструктора: имя → callback data роута (allow-list; тест сверяет с каталогом).
var routeData = map[string]string{
	"menu": routeMenu, "info": routeInfo, "pay": routePay, "promo": routePromo, "devices": routeDevices,
	"trial": routeTrial, "gift": routeGift, "referral": routeReferral, "devices_buy": routeDevicesBuy, "devices_clear": routeDevicesClear,
}

// argKey — какую кнопку экрана питает привязанное действие (flow/copy): ключ в b.args.
var argKey = map[string]string{
	"flow:yes": "button.yes", "flow:no": "button.no", "flow:have_promo": "button.have_promo",
	"link:login": "button.login", "flow:back": "button.back", "flow:devices_custom": "button.devices_custom", "flow:drop_devices": "button.drop_devices", "flow:remove_promo": "button.remove_promo",
	"copy:subscription": "button.subscription", "copy:referral_link": "button.copy_link", "copy:gift_link": "button.copy_link",
}

// templateBlock — какой блок экрана раскрывает шаблон.
var templateBlock = map[string]string{"tariffs": "@tariffs", "gift_tariffs": "@tariffs", "devices": "@devices"}

func (b *kbBuilder) fromLayout(lay texts.MsgLayout) *models.InlineKeyboardMarkup {
	def, _, _ := texts.MessageDefault(b.key)
	defStyle := map[string]keyboard.Style{}
	for _, row := range def.Rows {
		for _, c := range row {
			defStyle[c.ID] = keyboard.Style(c.Style)
		}
	}
	var rows [][]keyboard.Button
	for _, row := range lay.Rows {
		var cur []keyboard.Button
		for _, cell := range row {
			if cell.IsTemplate() {
				if len(cur) > 0 {
					rows, cur = append(rows, cur), nil
				}
				rows = append(rows, b.built[templateBlock[cell.Template()]]...)
				continue
			}
			if btn, ok := b.cell(cell, defStyle[cell.ID]); ok {
				cur = append(cur, btn)
			}
		}
		if len(cur) > 0 {
			rows = append(rows, cur)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return keyboard.Inline(rows...)
}

// cell строит кнопку конструктора; false — не показывать (условия, встроенное ограничение действия, экран не
// передал данных для действия).
func (b *kbBuilder) cell(c texts.Cell, defStyle keyboard.Style) (keyboard.Button, bool) {
	for _, cr := range c.Conditions {
		if b.env.eval(cr.ID) == cr.Not {
			return keyboard.Button{}, false
		}
	}
	label := b.c.Button(b.ctx, c.Key)
	if c.Label != "" {
		label = b.c.Label(b.ctx, c.Label)
	}
	style := defStyle
	switch {
	case c.Style != "":
		style = keyboard.Style(c.Style)
	case c.Key != "":
		style = b.c.Style(b.ctx, c.Key, defStyle)
	}
	switch kind, name, _ := strings.Cut(c.Action, ":"); {
	case kind == "url":
		return keyboard.URL(label, name).Style(style), true
	case c.Action == "link:support":
		return keyboard.URL(label, b.c.Settings(b.ctx).SupportURL).Style(style), true
	case c.Action == "link:miniapp":
		if u := b.c.MiniAppURL(); u != "" {
			tab := ""
			if b.id == "devices" { // с экрана «Устройства» кабинет открывается на устройствах
				tab = "devices"
			}
			return keyboard.WebApp(label, miniAppURL(u, tab)).Style(style), true
		}
		return keyboard.Button{}, false
	case c.Action == "link:login": // ссылка с одноразовым токеном знает только экран входа
		v, ok := b.args[argKey[c.Action]]
		if !ok {
			return keyboard.Button{}, false
		}
		return keyboard.URL(label, v).Style(style), true
	case kind == "route":
		if a, _ := texts.LookupAction(c.Action); a.Guard != "" && !b.env.eval(a.Guard) {
			return keyboard.Button{}, false
		}
		data, ok := routeData[name]
		if !ok {
			return keyboard.Button{}, false
		}
		return keyboard.Callback(label, data).Style(style), true
	case kind == "flow" || kind == "copy":
		v, ok := b.args[argKey[c.Action]]
		if !ok {
			return keyboard.Button{}, false
		}
		if kind == "copy" {
			return keyboard.Copy(label, v).Style(style), true
		}
		return keyboard.Callback(label, v).Style(style), true
	}
	return keyboard.Button{}, false
}
