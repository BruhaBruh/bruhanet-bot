package routes

import (
	"context"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
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
