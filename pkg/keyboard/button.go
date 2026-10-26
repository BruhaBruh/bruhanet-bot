package keyboard

import "github.com/go-telegram/bot/models"

type Style string

const (
	StyleDefault Style = ""
	StylePrimary Style = "primary"
	StyleSuccess Style = "success"
	StyleDanger  Style = "danger"
)

type kind int

const (
	kindCallback kind = iota
	kindURL
	kindCopy
)

type Button struct {
	kind  kind
	text  string
	style Style
	data  string
	url   string
	copy  string
}

func Callback(text, data string) Button {
	return Button{kind: kindCallback, text: text, data: data}
}

func URL(text, url string) Button {
	return Button{kind: kindURL, text: text, url: url}
}

func Copy(text, value string) Button {
	return Button{kind: kindCopy, text: text, copy: value}
}

func (b Button) Style(s Style) Button {
	b.style = s
	return b
}

func (b Button) build() models.InlineKeyboardButton {
	out := models.InlineKeyboardButton{Text: b.text, Style: string(b.style)}
	switch b.kind {
	case kindCallback:
		out.CallbackData = b.data
	case kindURL:
		out.URL = b.url
	case kindCopy:
		out.CopyText = &models.CopyTextButton{Text: b.copy}
	}
	return out
}
