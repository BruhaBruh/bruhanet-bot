package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
	// Kind: link — обычная https-ссылка (магазин, релизы); subscription — схема приложения с плейсхолдером
	// {{SUBSCRIPTION_LINK}}, ссылку подписки подставляет клиент по нажатию.
	Kind string `json:"kind"`
}

// instrBlock — шаг инструкции: заголовок, описание, значок и кнопки (как блок на странице подписки).
type instrBlock struct {
	Title   string        `json:"title"`
	Text    string        `json:"text"`
	Icon    string        `json:"icon"` // download|cloud|gear|check|""
	Tone    string        `json:"tone"` // cyan|teal|red|violet
	Buttons []instrButton `json:"buttons"`
}

type instrApp struct {
	Name     string       `json:"name"`
	Featured bool         `json:"featured"`
	Blocks   []instrBlock `json:"blocks"`
}

type instrPlatform struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Apps  []instrApp `json:"apps"`
}

// instructionsDTO — инструкция подключения без ссылки подписки: плейсхолдер {{SUBSCRIPTION_LINK}} остаётся в шаблонах
// кнопок, клиент подставляет ссылку, которую получил по явному нажатию.
type instructionsDTO struct {
	Platforms []instrPlatform `json:"platforms"`
}

const subscriptionPlaceholder = "{{SUBSCRIPTION_LINK}}"

var (
	platformOrder = []string{"ios", "android", "windows", "macos", "linux", "androidTV", "appleTV"}
	platformNames = map[string]string{
		"ios": "iOS", "android": "Android", "windows": "Windows", "macos": "macOS", "linux": "Linux",
		"androidTV": "Android TV", "appleTV": "Apple TV",
	}
	blockIcons = map[string]string{"DownloadIcon": "download", "CloudDownload": "cloud", "Gear": "gear", "Check": "check"}
	blockTones = []string{"cyan", "teal", "red", "violet"}
	appScheme  = regexp.MustCompile(`^[a-z][a-z0-9+.-]{1,31}://`)
	// В subscriptionLink к схеме приложения клиент подставляет ссылку подписки, поэтому схема — только у известных
	// VPN-клиентов (allowlist, не denylist). Новый клиент в конфиге панели = новая строка здесь.
	clientSchemes = []string{
		"happ", "v2box", "v2raytun", "v2rayng", "v2rayn", "v2rayu", "sing-box", "singbox", "clash", "clashmeta", "clash-verge",
		"hiddify", "streisand", "karing", "flclash", "flclashx", "koala-clash", "prizrak-box", "shadowrocket", "foxray",
		"nekoray", "nekobox", "sub", "stash", "loon", "surge", "quantumult-x", "v2rayx", "mihomo", "vpn4tv",
	}
)

// safeHTTPS — ссылка вида https://хост/…: без пробелов и управляющих символов, до 512 байт.
func safeHTTPS(u string) bool {
	return len(u) > len("https://") && len(u) <= 512 && strings.HasPrefix(u, "https://") &&
		!strings.ContainsFunc(u, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// subscriptionTemplate проверяет шаблон кнопки «Добавить подписку»: схема из allowlist, плейсхолдер подписки ровно один,
// {{USERNAME}} заменяется на экранированное имя; любой другой плейсхолдер — отказ.
func subscriptionTemplate(link, username string) (string, bool) {
	if len(link) > 512 || !appScheme.MatchString(link) || strings.Count(link, subscriptionPlaceholder) != 1 ||
		strings.ContainsFunc(link, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", false
	}
	scheme, _, _ := strings.Cut(link, ":")
	if !slices.Contains(clientSchemes, scheme) {
		return "", false
	}
	link = strings.ReplaceAll(link, "{{USERNAME}}", url.QueryEscape(username))
	if strings.Contains(strings.ReplaceAll(link, subscriptionPlaceholder, ""), "{{") {
		return "", false
	}
	return link, true
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
// фильтр ссылок. Неизвестное отбрасывается; пустой или чужой формат — пустой список платформ. username подставляется
// в {{USERNAME}} шаблонов кнопок.
func sanitizeInstructions(raw []byte, username string) (instructionsDTO, error) {
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
			if app, ok := sanitizeApp(obj(a), username); ok {
				pl.Apps = append(pl.Apps, app)
			}
		}
		if len(pl.Apps) > 0 {
			out.Platforms = append(out.Platforms, pl)
		}
	}
	return out, nil
}

func sanitizeApp(a map[string]any, username string) (instrApp, bool) {
	name := localized(a["name"], instrMaxLabel)
	if a == nil || name == "" {
		return instrApp{}, false
	}
	app := instrApp{Name: name, Blocks: []instrBlock{}}
	app.Featured, _ = a["featured"].(bool)
	blocks, _ := a["blocks"].([]any)
	for _, b := range blocks {
		if len(app.Blocks) >= 8 {
			break
		}
		bm := obj(b)
		if bm == nil {
			continue
		}
		block := instrBlock{
			Title: localized(bm["title"], instrMaxLabel), Text: localized(bm["description"], instrMaxText),
			Icon: blockIcons[fmt.Sprint(bm["svgIconKey"])], Tone: "cyan", Buttons: []instrButton{},
		}
		if tone, _ := bm["svgIconColor"].(string); slices.Contains(blockTones, tone) {
			block.Tone = tone
		}
		btns, _ := bm["buttons"].([]any)
		for _, bt := range btns {
			if len(block.Buttons) >= 6 {
				break
			}
			btm := obj(bt)
			link, _ := btm["link"].(string)
			text := localized(btm["text"], instrMaxLabel)
			if btm == nil || text == "" {
				continue
			}
			switch kind, _ := btm["type"].(string); kind {
			case "external":
				if safeHTTPS(link) {
					block.Buttons = append(block.Buttons, instrButton{Text: text, URL: link, Kind: "link"})
				}
			case "subscriptionLink":
				if tpl, ok := subscriptionTemplate(link, username); ok {
					block.Buttons = append(block.Buttons, instrButton{Text: text, URL: tpl, Kind: "subscription"})
				}
			}
		}
		if block.Title != "" || block.Text != "" || len(block.Buttons) > 0 {
			app.Blocks = append(app.Blocks, block)
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
	dto, err := sanitizeInstructions(raw, info.Username)
	if err != nil {
		stlog.L(r.Context()).Warn("mini app instructions unparsable", slog.Any("error", err))
		writeJSON(w, http.StatusOK, empty)
		return
	}
	s.instr.put(info.ShortUuid, dto, s.Now())
	writeJSON(w, http.StatusOK, dto)
}
