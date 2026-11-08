package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	instrTTL      = 10 * time.Minute
	instrMaxKeys  = 500
	instrMaxText  = 2000
	instrMaxLabel = 64
)

type instrButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type instrStep struct {
	Title   string        `json:"title"`
	Text    string        `json:"text"`
	Buttons []instrButton `json:"buttons"`
}

type instrApp struct {
	Name       string      `json:"name"`
	Featured   bool        `json:"featured"`
	URLScheme  string      `json:"urlScheme"`
	NeedBase64 bool        `json:"needBase64"`
	Steps      []instrStep `json:"steps"`
}

type instrPlatform struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Apps  []instrApp `json:"apps"`
}

// instructionsDTO — инструкция подключения без ссылки подписки: шаблон {{SUBSCRIPTION_LINK}} остаётся плейсхолдером,
// а схему приложения (urlScheme) клиент достраивает ссылкой, которую получил по явному нажатию.
type instructionsDTO struct {
	Platforms []instrPlatform `json:"platforms"`
}

var (
	platformOrder = []string{"ios", "android", "windows", "macos", "linux", "androidTV", "appleTV"}
	platformNames = map[string]string{
		"ios": "iOS", "android": "Android", "windows": "Windows", "macos": "macOS", "linux": "Linux",
		"androidTV": "Android TV", "appleTV": "Apple TV",
	}
	// ключ шага в конфиге панели → заголовок
	stepKeys = []struct{ key, title string }{
		{"installationStep", "Установка"},
		{"additionalBeforeAddSubscriptionStep", "Перед добавлением"},
		{"addSubscriptionStep", "Добавление подписки"},
		{"additionalAfterAddSubscriptionStep", "После добавления"},
		{"connectAndUseStep", "Подключение"},
	}
	appScheme     = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,31}://`)
	forbiddenSchm = []string{"javascript", "data", "file", "vbscript", "blob", "about", "http"}
)

// safeLink пропускает https:// и схемы приложений (happ://, v2box://…); javascript:/data:/file:/http: и прочее — нет.
func safeLink(u string) bool {
	if u == "" || len(u) > 512 || strings.ContainsFunc(u, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return false
	}
	if strings.HasPrefix(u, "https://") {
		return len(u) > len("https://")
	}
	if !appScheme.MatchString(u) {
		return false
	}
	scheme, _, _ := strings.Cut(u, ":")
	return !slices.Contains(forbiddenSchm, scheme)
}

// localized достаёт русский (иначе английский, иначе первый) текст из {"ru": "...", "en": "..."} или обычной строки.
func localized(v any, limit int) string {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case map[string]any:
		for _, k := range []string{"ru", "en"} {
			if x, ok := t[k].(string); ok && strings.TrimSpace(x) != "" {
				s = x
				break
			}
		}
		if s == "" {
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				if x, ok := t[k].(string); ok && strings.TrimSpace(x) != "" {
					s = x
					break
				}
			}
		}
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > limit {
		s = string(r[:limit])
	}
	return s
}

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }

// sanitizeInstructions переводит сырой конфиг страницы подписки в безопасный DTO: известные поля, лимиты на размер,
// фильтр ссылок. Неизвестное отбрасывается; пустой или чужой формат — пустой список платформ.
func sanitizeInstructions(raw []byte) (instructionsDTO, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return instructionsDTO{}, errors.Join(errors.New("subpage config is not an object"), err)
	}
	platforms := obj(root["platforms"])
	ids := make([]string, 0, len(platforms))
	for _, id := range platformOrder {
		if _, ok := platforms[id]; ok {
			ids = append(ids, id)
		}
	}
	var rest []string
	for id := range platforms {
		if !slices.Contains(platformOrder, id) && len(id) <= 32 {
			rest = append(rest, id)
		}
	}
	slices.Sort(rest)
	ids = append(ids, rest...)

	out := instructionsDTO{Platforms: []instrPlatform{}}
	for _, id := range ids {
		if len(out.Platforms) >= 12 {
			break
		}
		p := obj(platforms[id])
		if p == nil {
			continue
		}
		title := localized(p["displayName"], instrMaxLabel)
		if title == "" {
			if title = platformNames[id]; title == "" {
				title = id
			}
		}
		pl := instrPlatform{ID: id, Title: title, Apps: []instrApp{}}
		apps, _ := p["apps"].([]any)
		for _, a := range apps {
			if len(pl.Apps) >= 10 {
				break
			}
			if app, ok := sanitizeApp(obj(a)); ok {
				pl.Apps = append(pl.Apps, app)
			}
		}
		if len(pl.Apps) > 0 {
			out.Platforms = append(out.Platforms, pl)
		}
	}
	return out, nil
}

func sanitizeApp(a map[string]any) (instrApp, bool) {
	name := localized(a["name"], instrMaxLabel)
	if a == nil || name == "" {
		return instrApp{}, false
	}
	app := instrApp{Name: name, Steps: []instrStep{}}
	app.Featured, _ = a["featured"].(bool)
	app.NeedBase64, _ = a["isNeedBase64Encoding"].(bool)
	if sch, _ := a["urlScheme"].(string); safeLink(sch) && !strings.HasPrefix(sch, "https://") {
		app.URLScheme = sch
	}
	for _, sk := range stepKeys {
		st := obj(a[sk.key])
		if st == nil {
			continue
		}
		step := instrStep{Title: sk.title, Text: localized(st["description"], instrMaxText), Buttons: []instrButton{}}
		btns, _ := st["buttons"].([]any)
		for _, b := range btns {
			bm := obj(b)
			link, _ := bm["buttonLink"].(string)
			text := localized(bm["buttonText"], instrMaxLabel)
			if bm == nil || text == "" || !safeLink(link) || len(step.Buttons) >= 4 {
				continue
			}
			step.Buttons = append(step.Buttons, instrButton{Text: text, URL: link})
		}
		if step.Text != "" || len(step.Buttons) > 0 {
			app.Steps = append(app.Steps, step)
		}
	}
	return app, true
}

type instrEntry struct {
	dto instructionsDTO
	at  time.Time
}

// instrCache — разобранные инструкции по shortUuid, TTL instrTTL; при переполнении сбрасывается целиком.
type instrCache struct {
	mu sync.Mutex
	m  map[string]instrEntry
}

func newInstrCache() *instrCache { return &instrCache{m: map[string]instrEntry{}} }

func (c *instrCache) get(key string, now time.Time) (instructionsDTO, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || now.Sub(e.at) >= instrTTL {
		return instructionsDTO{}, false
	}
	return e.dto, true
}

func (c *instrCache) put(key string, dto instructionsDTO, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= instrMaxKeys {
		clear(c.m)
	}
	c.m[key] = instrEntry{dto, now}
}

// miniInstructions — инструкция подключения из конфига страницы подписки Remnawave. Нет юзера в панели, конфиг не
// задан или не разобрался — пустой список платформ (клиент покажет «скопируй ссылку и добавь вручную»).
func (s *server) miniInstructions(w http.ResponseWriter, r *http.Request) {
	info, ok := s.miniPanelUser(w, r)
	if !ok {
		return
	}
	empty := instructionsDTO{Platforms: []instrPlatform{}}
	if info == nil || info.ShortUuid == "" {
		writeJSON(w, http.StatusOK, empty)
		return
	}
	if dto, hit := s.instr.get(info.ShortUuid, s.Now()); hit {
		writeJSON(w, http.StatusOK, dto)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	raw, err := s.Panel.SubpageConfig(ctx, info.ShortUuid)
	switch {
	case errors.Is(err, remnawave.ErrSubpageConfigNotSet), errors.Is(err, remnawave.ErrUserNotFound):
		writeJSON(w, http.StatusOK, empty)
		return
	case err != nil:
		panelFailed(w, r, "panel subpage config", err)
		return
	}
	dto, err := sanitizeInstructions(raw)
	if err != nil {
		stlog.L(r.Context()).Warn("mini app instructions unparsable", slog.Any("error", err))
		writeJSON(w, http.StatusOK, empty)
		return
	}
	s.instr.put(info.ShortUuid, dto, s.Now())
	writeJSON(w, http.StatusOK, dto)
}
