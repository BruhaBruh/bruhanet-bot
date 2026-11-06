package texts

import (
	"html"
	"regexp"
	"slices"
	"strings"
	"unicode/utf16"
)

var placeholder = regexp.MustCompile(`\{\{\s*\.([^{}\s]+)\s*\}\}`)

func replaceVars(tpl string, value func(path string) string) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		return value(placeholder.FindStringSubmatch(m)[1])
	})
}

// Render подставляет {{.path}}: значение из vars HTML-экранируется, неизвестная переменная — пусто.
func Render(tpl string, vars map[string]string) string {
	return RenderRaw(tpl, vars, nil)
}

// RenderRaw как Render, но значения из raw — готовый HTML и вставляются как есть.
func RenderRaw(tpl string, vars, raw map[string]string) string {
	return replaceVars(tpl, func(path string) string {
		if v, ok := raw[path]; ok {
			return v
		}
		if v, ok := vars[path]; ok {
			return html.EscapeString(v)
		}
		return ""
	})
}

// RenderPlain — подстановка без экранирования: для кнопок и простых текстов.
func RenderPlain(tpl string, vars map[string]string) string {
	return replaceVars(tpl, func(path string) string {
		if v, ok := vars[path]; ok {
			return v
		}
		return ""
	})
}

// HasVars — в тексте есть {{.path}}.
func HasVars(body string) bool { return placeholder.MatchString(body) }

// Uses — текст использует переменную name.
func Uses(body, name string) bool {
	return slices.ContainsFunc(placeholder.FindAllStringSubmatch(body, -1), func(m []string) bool { return m[1] == name })
}

// Blank — после рендера не осталось видимого текста: Telegram такое не примет.
func Blank(rendered string) bool {
	text, err := checkMarkup(rendered)
	return err == nil && strings.TrimSpace(text) == ""
}

// FitsPlain — подпись кнопки или простой текст годится для Telegram: не пустая и не длиннее лимита.
func FitsPlain(s string) bool {
	return strings.TrimSpace(s) != "" && utf16Len(s) <= maxButtonLen
}

// ClipPlain обрезает до лимита кнопки; последняя страховка, когда годного текста нет.
func ClipPlain(s string) string { return clip(s, maxButtonLen) }

// clip обрезает до n UTF-16 units.
func clip(s string, n int) string {
	used := 0
	for i, r := range s {
		used += max(utf16.RuneLen(r), 1)
		if used > n {
			return s[:i]
		}
	}
	return s
}

// RenderDef рендерит текст по его виду: в сообщении Raw-переменные def.Vars вставляются как есть,
// остальные экранируются; кнопки и простые тексты — без экранирования и без обрезки: что поместится в лимит,
// проверяет Validate при сохранении, а случайный выход за него ловит вызывающий через FitsPlain.
func RenderDef(def Def, body string, vars map[string]string) string {
	if def.Kind != KindMessage {
		return RenderPlain(body, vars)
	}
	raw := map[string]string{}
	for _, v := range def.Vars {
		if val, ok := vars[v.Name]; ok && v.Raw {
			raw[v.Name] = val
		}
	}
	return RenderRaw(body, vars, raw)
}

// Samples — значения-примеры общих переменных, своих (custom: имя → значение) и переменных текста.
func Samples(def Def, custom map[string]string) map[string]string {
	all := append(Globals(custom), def.Vars...)
	out := make(map[string]string, len(all))
	for _, v := range all {
		out[v.Name] = v.Sample
	}
	return out
}

// maxSamples — как Samples, но каждое значение дополнено до Max (худший случай); Raw — в <code>.
// Пример остаётся началом значения: ссылка в href после дополнения всё ещё проходит проверку.
func maxSamples(def Def, custom map[string]string) map[string]string {
	out := Samples(def, custom)
	for _, v := range append(Globals(custom), def.Vars...) {
		if v.Max <= utf16Len(v.Sample) {
			continue
		}
		if v.Raw {
			out[v.Name] = "<code>" + strings.Repeat("я", v.Max) + "</code>"
			continue
		}
		out[v.Name] = v.Sample + strings.Repeat("я", v.Max-utf16Len(v.Sample))
	}
	return out
}

// WorstLen — видимая длина текста, когда все переменные максимальной длины.
func WorstLen(def Def, body string, custom map[string]string) int {
	return VisibleLen(RenderDef(def, body, maxSamples(def, custom)))
}
