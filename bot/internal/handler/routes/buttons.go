package routes

import (
	"context"

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

func copyButton(ctx context.Context, c *content.Provider, key, value string, style keyboard.Style) keyboard.Button {
	return keyboard.Copy(c.Button(ctx, key), value).Style(c.Style(ctx, key, style))
}

// kbBuilder собирает клавиатуру зарегистрированной раскладки (texts.Keyboard): код создаёт кнопки по условиям, а ряды и
// порядок решает раскладка — сохранённая из админки или по умолчанию. Кнопки, которых код не создал, в раскладке
// пропадают, а созданные, которых в сохранённой раскладке нет, встают в конец.
type kbBuilder struct {
	ctx   context.Context
	c     *content.Provider
	id    string
	built map[string][][]keyboard.Button
}

func newKB(ctx context.Context, c *content.Provider, id string) *kbBuilder {
	return &kbBuilder{ctx: ctx, c: c, id: id, built: map[string][][]keyboard.Button{}}
}

func (b *kbBuilder) put(key string, btn keyboard.Button) *kbBuilder {
	b.built[key] = [][]keyboard.Button{{btn}}
	return b
}

func (b *kbBuilder) callback(key, data string, style keyboard.Style) *kbBuilder {
	return b.put(key, callbackButton(b.ctx, b.c, key, data, style))
}

func (b *kbBuilder) url(key, url string, style keyboard.Style) *kbBuilder {
	return b.put(key, urlButton(b.ctx, b.c, key, url, style))
}

func (b *kbBuilder) copy(key, value string, style keyboard.Style) *kbBuilder {
	return b.put(key, copyButton(b.ctx, b.c, key, value, style))
}

// block кладёт динамический блок (`@…`): его ряды остаются рядами.
func (b *kbBuilder) block(id string, rows [][]keyboard.Button) *kbBuilder {
	b.built[id] = rows
	return b
}

// markup — разметка по действующей раскладке; все кнопки скрыты — nil (сообщение без клавиатуры).
func (b *kbBuilder) markup() *models.InlineKeyboardMarkup {
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
