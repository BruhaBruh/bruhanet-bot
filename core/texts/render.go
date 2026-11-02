package texts

import (
	"html"
	"regexp"
)

var placeholder = regexp.MustCompile(`\{\{\s*\.([^{}\s]+)\s*\}\}`)

// Render подставляет {{.path}}: значение из vars HTML-экранируется, неизвестная переменная — её путь.
func Render(tpl string, vars map[string]string) string {
	return RenderRaw(tpl, vars, nil)
}

// RenderRaw как Render, но значения из raw — готовый HTML и вставляются как есть.
func RenderRaw(tpl string, vars, raw map[string]string) string {
	return placeholder.ReplaceAllStringFunc(tpl, func(m string) string {
		path := placeholder.FindStringSubmatch(m)[1]
		if v, ok := raw[path]; ok {
			return v
		}
		if v, ok := vars[path]; ok {
			return html.EscapeString(v)
		}
		return html.EscapeString(path)
	})
}

// Samples — значения-примеры всех переменных текста.
func Samples(def Def) map[string]string {
	out := make(map[string]string, len(def.Vars))
	for _, v := range def.Vars {
		out[v.Name] = v.Sample
	}
	return out
}

func renderSamples(def Def, body string) string {
	vars := map[string]string{}
	raw := map[string]string{}
	for _, v := range def.Vars {
		if v.Raw {
			raw[v.Name] = v.Sample
		} else {
			vars[v.Name] = v.Sample
		}
	}
	return RenderRaw(body, vars, raw)
}
