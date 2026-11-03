package texts

import (
	"html"
	"regexp"
	"strings"
)

var placeholder = regexp.MustCompile(`\{\{\s*\.([^{}\s]+)\s*\}\}`)

func replaceVars(tpl string, value func(path string) string) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		return value(placeholder.FindStringSubmatch(m)[1])
	})
}

// Render подставляет {{.path}}: значение из vars HTML-экранируется, неизвестная переменная — её путь.
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
		return html.EscapeString(path)
	})
}

// RenderPlain — подстановка без экранирования: для кнопок и простых текстов.
func RenderPlain(tpl string, vars map[string]string) string {
	return replaceVars(tpl, func(path string) string {
		if v, ok := vars[path]; ok {
			return v
		}
		return path
	})
}

// RenderDef рендерит текст по его виду: в сообщении Raw-переменные def.Vars вставляются как есть,
// остальные экранируются; кнопки и простые тексты — без экранирования.
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

// Samples — значения-примеры всех переменных текста.
func Samples(def Def) map[string]string {
	out := make(map[string]string, len(def.Vars))
	for _, v := range def.Vars {
		out[v.Name] = v.Sample
	}
	return out
}

// maxSamples — как Samples, но каждое значение дополнено до Max (худший случай); Raw — в <code>.
func maxSamples(def Def) map[string]string {
	out := Samples(def)
	for _, v := range def.Vars {
		if v.Max <= utf16Len(v.Sample) {
			continue
		}
		out[v.Name] = strings.Repeat("я", v.Max)
		if v.Raw {
			out[v.Name] = "<code>" + out[v.Name] + "</code>"
		}
	}
	return out
}
