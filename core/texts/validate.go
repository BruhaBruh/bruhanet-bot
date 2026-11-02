package texts

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	maxMessageLen = 4096
	maxButtonLen  = 64
)

// allowedTags — теги Telegram HTML и разрешённые им атрибуты.
var allowedTags = map[string][]string{
	"b": nil, "strong": nil, "i": nil, "em": nil, "u": nil, "ins": nil,
	"s": nil, "strike": nil, "del": nil, "code": nil, "pre": nil,
	"tg-spoiler": nil, "blockquote": nil,
	"a":    {"href"},
	"span": {"class"},
}

// Validate проверяет текст def после рендера с примерами переменных.
func Validate(def Def, body string) error {
	rendered := renderSamples(def, body)
	if def.Kind == KindButton {
		return validateButton(rendered)
	}
	n, err := checkMarkup(rendered)
	if err != nil {
		return errors.Join(ErrInvalidMarkup, err)
	}
	if n > maxMessageLen {
		return errors.Join(ErrTooLong, fmt.Errorf("текст длиннее %d символов (%d)", maxMessageLen, n))
	}
	return nil
}

func validateButton(s string) error {
	switch {
	case strings.TrimSpace(s) == "":
		return errors.Join(ErrBadButton, errors.New("текст кнопки пустой"))
	case strings.ContainsAny(s, "<>"):
		return errors.Join(ErrBadButton, errors.New("в тексте кнопки нельзя использовать теги"))
	case utf8.RuneCountInString(s) > maxButtonLen:
		return errors.Join(ErrBadButton, fmt.Errorf("текст кнопки длиннее %d символов", maxButtonLen))
	}
	return nil
}

// checkMarkup проверяет теги, атрибуты и вложенность; возвращает длину видимого текста.
func checkMarkup(s string) (int, error) {
	z := html.NewTokenizer(strings.NewReader(s))
	var open []string
	n := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				return 0, z.Err()
			}
			if len(open) > 0 {
				return 0, fmt.Errorf("тег <%s> не закрыт", open[len(open)-1])
			}
			return n, nil
		case html.TextToken:
			if strings.ContainsAny(string(z.Raw()), "<>") {
				return 0, errors.New("символы < и > вне тегов нужно писать как &lt; и &gt;")
			}
			n += utf8.RuneCount(z.Text())
		case html.StartTagToken:
			tok := z.Token()
			if err := checkTag(tok); err != nil {
				return 0, err
			}
			open = append(open, tok.Data)
		case html.EndTagToken:
			name := z.Token().Data
			if len(open) == 0 || open[len(open)-1] != name {
				return 0, fmt.Errorf("лишний или неправильно вложенный тег </%s>", name)
			}
			open = open[:len(open)-1]
		case html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
			return 0, fmt.Errorf("недопустимая разметка %q", z.Raw())
		}
	}
}

func checkTag(tok html.Token) error {
	attrs, ok := allowedTags[tok.Data]
	if !ok {
		return fmt.Errorf("тег <%s> не поддерживается Telegram", tok.Data)
	}
	for _, a := range tok.Attr {
		if a.Namespace != "" || !slices.Contains(attrs, a.Key) {
			return fmt.Errorf("атрибут %s у тега <%s> не поддерживается", a.Key, tok.Data)
		}
	}
	if tok.Data == "span" && (len(tok.Attr) != 1 || tok.Attr[0].Val != "tg-spoiler") {
		return errors.New(`тег <span> допустим только как <span class="tg-spoiler">`)
	}
	if tok.Data == "a" && (len(tok.Attr) != 1 || tok.Attr[0].Val == "") {
		return errors.New("у ссылки <a> нужен атрибут href")
	}
	return nil
}
