package texts

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"

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

var allowedSchemes = []string{"http", "https", "tg", "mailto"}

// ampersand находит каждое & — либо в составе допустимой в Telegram сущности, либо голое.
var ampersand = regexp.MustCompile(`&(?:lt|gt|amp|quot|#[0-9]+|#[xX][0-9a-fA-F]+);|&`)

// malformedVar — похоже на переменную, но не разбирается: {{name}}, {{.a b}}, {{.name}. Голые «{{» — обычный текст.
var malformedVar = regexp.MustCompile(`\{\{[^{}]*\}\}|\{\{\s*\.`)

func checkVars(def Def, body string, custom map[string]string) error {
	if m := malformedVar.FindString(placeholder.ReplaceAllString(body, "")); m != "" {
		return fmt.Errorf("неправильная переменная %s: нужно {{.имя}}, целиком и без форматирования внутри", m)
	}
	known := Samples(def, custom)
	for _, m := range placeholder.FindAllStringSubmatch(body, -1) {
		if _, ok := known[m[1]]; !ok {
			return fmt.Errorf("неизвестная переменная %s", m[1])
		}
	}
	return nil
}

// Validate проверяет текст def после рендера с примерами переменных; custom — свои переменные (имя → значение).
func Validate(def Def, body string, custom map[string]string) error {
	sentinel := ErrInvalidMarkup
	if def.Kind != KindMessage {
		sentinel = ErrBadButton
	}
	if err := checkVars(def, body, custom); err != nil {
		return errors.Join(sentinel, err)
	}
	if def.Kind != KindMessage && strings.ContainsAny(body, "<>") {
		return errors.Join(ErrBadButton, errors.New("в этом тексте нельзя использовать теги"))
	}
	rendered := RenderDef(def, body, Samples(def, custom))
	if def.Kind != KindMessage {
		return validatePlain(def, body, rendered, custom)
	}
	text, err := checkMarkup(rendered)
	if err != nil {
		return errors.Join(ErrInvalidMarkup, err)
	}
	if strings.TrimSpace(text) == "" {
		return errors.Join(ErrInvalidMarkup, errors.New("текст не может быть пустым"))
	}
	if n := utf16Len(text); n > maxMessageLen {
		return errors.Join(ErrTooLong, fmt.Errorf("текст длиннее %d символов (%d)", maxMessageLen, n))
	}
	// худший случай: все переменные максимальной длины
	worst, err := checkMarkup(RenderDef(def, body, maxSamples(def, custom)))
	if err != nil {
		return errors.Join(ErrInvalidMarkup, err)
	}
	if n := utf16Len(worst); n > maxMessageLen {
		return errors.Join(ErrTooLong, fmt.Errorf("текст может оказаться длиннее %d символов (с самыми длинными значениями переменных — %d); самая длинная — {{.%s}}", maxMessageLen, n, WidestVar(def, body, custom)))
	}
	return nil
}

// ValidateStored — проверка уже сохранённого текста при загрузке: переменная, которой больше нет
// (своя удалена, настройки откатились на env), не отменяет правку — она отрендерится пустой.
func ValidateStored(def Def, body string, custom map[string]string) error {
	known := Samples(def, custom)
	all := maps.Clone(custom)
	if all == nil {
		all = map[string]string{}
	}
	for _, m := range placeholder.FindAllStringSubmatch(body, -1) {
		if _, ok := known[m[1]]; !ok {
			all[m[1]] = "x"
		}
	}
	return Validate(def, body, all)
}

func varLens(def Def, custom map[string]string, byValue bool) map[string]int {
	lens := map[string]int{}
	for _, v := range append(Globals(custom), def.Vars...) {
		lens[v.Name] = max(v.Max, utf16Len(v.Sample))
	}
	if byValue {
		for name, val := range custom {
			lens[name] = utf16Len(val) // своя переменная в кнопке: её значение известно
		}
	}
	return lens
}

// WidestVar — имя самой длинной (в худшем случае) переменной из body; пусто, если переменных нет.
func WidestVar(def Def, body string, custom map[string]string) string {
	lens := varLens(def, custom, def.Kind != KindMessage)
	widest, widestLen := "", -1
	for _, m := range placeholder.FindAllStringSubmatch(body, -1) {
		if n := lens[m[1]]; n > widestLen {
			widest, widestLen = m[1], n
		}
	}
	return widest
}

// validatePlain: рендер на примерах не пуст, а худший случай (переменные максимальной длины, свои — по текущему
// значению) помещается в лимит; иначе ошибка называет самую длинную переменную.
func validatePlain(def Def, body, rendered string, custom map[string]string) error {
	if strings.TrimSpace(rendered) == "" {
		return errors.Join(ErrBadButton, errors.New("текст не может быть пустым"))
	}
	fixed := utf16Len(placeholder.ReplaceAllString(body, ""))
	if fixed > maxButtonLen {
		return errors.Join(ErrBadButton, fmt.Errorf("текст длиннее %d символов", maxButtonLen))
	}
	total, widest := fixed, WidestVar(def, body, custom)
	lens := varLens(def, custom, true)
	for _, m := range placeholder.FindAllStringSubmatch(body, -1) {
		total += lens[m[1]]
	}
	if total > maxButtonLen {
		where := "в кнопку"
		if def.Kind == KindPlain {
			where = "в текст"
		}
		return errors.Join(ErrBadButton, fmt.Errorf("{{.%s}} может не влезть %s: до %d символов (с самым длинным значением переменных — %d)", widest, where, maxButtonLen, total))
	}
	return nil
}

// VisibleLen — длина видимого текста Telegram HTML в UTF-16 (как считает Telegram);
// для невалидной разметки — длина как есть. Вызывать после Validate.
func VisibleLen(body string) int {
	if text, err := checkMarkup(body); err == nil {
		return utf16Len(text)
	}
	return utf16Len(body)
}

// utf16Len — длина в UTF-16 code units, как её считает Telegram.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += max(utf16.RuneLen(r), 1)
	}
	return n
}

// checkMarkup проверяет сущности, теги, атрибуты и вложенность; возвращает видимый текст.
func checkMarkup(s string) (string, error) {
	for _, m := range ampersand.FindAllString(s, -1) {
		if m == "&" {
			return "", errors.New("символ & нужно писать как &amp;, допустимы только &lt; &gt; &amp; &quot; и числовые сущности")
		}
	}
	z := html.NewTokenizer(strings.NewReader(s))
	var open []string
	var text strings.Builder
	for {
		switch z.Next() {
		case html.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				return "", z.Err()
			}
			if len(open) > 0 {
				return "", fmt.Errorf("тег <%s> не закрыт", open[len(open)-1])
			}
			return text.String(), nil
		case html.TextToken:
			if strings.ContainsAny(string(z.Raw()), "<>") {
				return "", errors.New("символы < и > вне тегов нужно писать как &lt; и &gt;")
			}
			text.Write(z.Text())
		case html.StartTagToken:
			tok := z.Token()
			if err := checkTag(tok); err != nil {
				return "", err
			}
			if err := checkNesting(open, tok.Data); err != nil {
				return "", err
			}
			open = append(open, tok.Data)
		case html.EndTagToken:
			name := z.Token().Data
			if len(open) == 0 || open[len(open)-1] != name {
				return "", fmt.Errorf("лишний или неправильно вложенный тег </%s>", name)
			}
			open = open[:len(open)-1]
		case html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
			return "", fmt.Errorf("недопустимая разметка %q", z.Raw())
		}
	}
}

// checkNesting: внутри code/pre тегов быть не может, blockquote и a не вкладываются сами в себя.
func checkNesting(open []string, tag string) error {
	if slices.ContainsFunc(open, func(o string) bool { return o == "code" || o == "pre" }) {
		return fmt.Errorf("внутри <%s> нельзя использовать теги", open[slices.IndexFunc(open, func(o string) bool { return o == "code" || o == "pre" })])
	}
	if (tag == "blockquote" || tag == "a") && slices.Contains(open, tag) {
		return fmt.Errorf("тег <%s> нельзя вкладывать в <%s>", tag, tag)
	}
	return nil
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
	switch tok.Data {
	case "span":
		if len(tok.Attr) != 1 || tok.Attr[0].Val != "tg-spoiler" {
			return errors.New(`тег <span> допустим только как <span class="tg-spoiler">`)
		}
	case "a":
		if len(tok.Attr) != 1 {
			return errors.New("у ссылки <a> нужен атрибут href")
		}
		u, err := url.Parse(tok.Attr[0].Val)
		if err != nil || !slices.Contains(allowedSchemes, strings.ToLower(u.Scheme)) {
			return fmt.Errorf("ссылка %q: допустимы только http, https, tg и mailto", tok.Attr[0].Val)
		}
		if (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")) && u.Host == "" {
			return fmt.Errorf("ссылка %q: не указан адрес сайта", tok.Attr[0].Val)
		}
	}
	return nil
}
