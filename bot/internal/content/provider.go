// Package content — тексты и настройки бота из БД (правки из админки) с кешем на ttl.
package content

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// Source — store.Store.
type Source interface {
	TextOverrides(ctx context.Context) (map[string]string, error)
	SettingsRaw(ctx context.Context) (map[string][]byte, error)
	UserContact(ctx context.Context, id int64) (model.UserContact, error)
}

// recipient — чьи данные подставлять в общие переменные текста; fromUpdate — имя уже известно из апдейта.
type recipient struct {
	id                  int64
	firstName, username string
	fromUpdate          bool
}

type recipientKey struct{}

// WithUser — автор апдейта: его имя есть в апдейте, срок читается из БД, только если текст его просит.
func WithUser(ctx context.Context, id int64, firstName, username string) context.Context {
	return context.WithValue(ctx, recipientKey{}, recipient{id: id, firstName: firstName, username: username, fromUpdate: true})
}

// ForUser — текст для другого юзера (не автора апдейта: админ решает платёж, напоминание, рассылка);
// данные читаются из БД, только если текст их просит.
func ForUser(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, recipientKey{}, recipient{id: id})
}

var (
	moscow     = loadMoscow()
	monthNames = [...]string{
		"", "января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря",
	}
)

func loadMoscow() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
}

// Moscow — зона показа времени юзеру.
func Moscow() *time.Location { return moscow }

// FormatTime — дата для юзера по МСК; значение {{.expireAt}}.
func FormatTime(t time.Time) string {
	t = t.In(moscow)
	zone, _ := t.Zone()
	return fmt.Sprintf("%d %s %d г. в %02d:%02d, %s",
		t.Day(), monthNames[t.Month()], t.Year(), t.Hour(), t.Minute(), zone)
}

type snapshot struct {
	texts    map[string]string // только прошедшие texts.Validate
	settings settings.Settings
}

// defaultRefreshTimeout — сколько ждать БД при обновлении кеша.
const defaultRefreshTimeout = 2 * time.Second

type Provider struct {
	src            Source
	defaults       settings.Settings
	ttl            time.Duration
	refreshTimeout time.Duration
	now            func() time.Time

	// снимок читается без блокировок; обновляет его ровно одна горутина
	snap atomic.Pointer[snapshot]

	mu         sync.Mutex // loadedAt, refreshing
	loadedAt   time.Time
	refreshing bool
	warned     map[string]string // key → тело, о невалидности которого уже предупредили; трогает только refresh
}

func New(src Source, defaults settings.Settings, ttl time.Duration, now func() time.Time) *Provider {
	if now == nil {
		now = time.Now
	}
	p := &Provider{src: src, defaults: defaults, ttl: ttl, refreshTimeout: defaultRefreshTimeout, now: now}
	p.snap.Store(&snapshot{settings: defaults})
	return p
}

// Text — переопределение из БД или текст по умолчанию, отрендеренный texts.RenderDef.
func (p *Provider) Text(ctx context.Context, key string, vars map[string]string) string {
	def, ok := texts.Lookup(key)
	if !ok {
		stlog.Error(ctx, "unknown text key", "key", key)
		return key
	}
	// один снимок на весь рендер: тело и значения своих переменных из одного поколения
	snap := p.current(ctx)
	body := def.Default
	if b, ok := snap.texts[key]; ok {
		body = b
	}
	out := texts.RenderDef(def, body, p.values(ctx, snap.settings, body, vars))
	if texts.Blank(out) { // все переменные пусты: Telegram отверг бы и сообщение, и всю клавиатуру
		out = texts.RenderDef(def, def.Default, p.values(ctx, snap.settings, def.Default, vars))
	}
	return out
}

// Render подставляет переменные в текст рассылки или личного сообщения для получателя id.
func (p *Provider) Render(ctx context.Context, id int64, tpl string) string {
	vals := p.values(ForUser(ctx, id), p.current(ctx).settings, tpl, nil)
	return texts.RenderDef(texts.Def{Kind: texts.KindMessage}, tpl, vals)
}

// values — значения переменных текста: общие из настроек и получателя, поверх — переданные вызывающим
// (он может знать точнее: только что продлённый срок). БД — только если тело просит то, чего нет в контексте.
func (p *Provider) values(ctx context.Context, s settings.Settings, body string, vars map[string]string) map[string]string {
	out := s.TextVars()
	r, _ := ctx.Value(recipientKey{}).(recipient)
	asks := func(name string) bool {
		_, given := vars[name]
		return !given && texts.Uses(body, name)
	}
	contact := model.UserContact{FirstName: r.firstName, Username: r.username}
	if r.id != 0 && (asks("expireAt") || (!r.fromUpdate && (asks("name") || asks("username")))) {
		cctx, cancel := context.WithTimeout(ctx, p.refreshTimeout)
		loaded, err := p.src.UserContact(cctx, r.id)
		cancel()
		if err != nil {
			stlog.Warn(ctx, "failed to load user for text", "error", err)
		}
		contact.ExpireAt = loaded.ExpireAt
		if !r.fromUpdate {
			contact.FirstName, contact.Username = loaded.FirstName, loaded.Username
		}
	}
	// ponytail: users.username бывает telegram id (telegramUsername для панели); уйдёт с багом этапа 3.
	if contact.Username == strconv.FormatInt(r.id, 10) {
		contact.Username = ""
	}
	nobody := "друг"
	if r.fromUpdate { // как в сообщении админу о платеже: автор апдейта без имени и username
		nobody = "#" + strconv.FormatInt(r.id, 10)
	}
	out["name"] = firstNonEmpty(contact.FirstName, contact.Username, nobody)
	out["username"] = firstNonEmpty(contact.Username, noValue)
	out["telegramId"] = noValue
	if r.id != 0 {
		out["telegramId"] = strconv.FormatInt(r.id, 10)
	}
	out["expireAt"] = noValue
	if contact.ExpireAt != nil {
		out["expireAt"] = FormatTime(*contact.ExpireAt)
	}
	maps.Copy(out, vars)
	return out
}

const noValue = "—"

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Button — подпись кнопки без переменных.
func (p *Provider) Button(ctx context.Context, key string) string {
	return p.Text(ctx, key, nil)
}

func (p *Provider) Settings(ctx context.Context) settings.Settings {
	return p.current(ctx).settings
}

// current отдаёт снимок; устаревший обновляет вызвавшая горутина (ждёт не дольше refreshTimeout),
// остальные в это время сразу получают прежний снимок (до первой загрузки — значения по умолчанию).
func (p *Provider) current(ctx context.Context) *snapshot {
	now := p.now()
	p.mu.Lock()
	stale := p.loadedAt.IsZero() || now.Sub(p.loadedAt) >= p.ttl
	if !stale || p.refreshing {
		p.mu.Unlock()
		return p.snap.Load()
	}
	p.refreshing = true
	p.mu.Unlock()

	defer func() { // и при панике: иначе refreshing навсегда останется true
		p.mu.Lock()
		p.loadedAt, p.refreshing = now, false
		p.mu.Unlock()
	}()
	p.refresh(ctx)
	return p.snap.Load()
}

// refresh перечитывает БД. Ошибка — остаются последние значения, следующая попытка через ttl.
func (p *Provider) refresh(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.refreshTimeout)
	defer cancel()
	next := *p.snap.Load()
	if raw, err := p.src.SettingsRaw(ctx); err != nil {
		stlog.Warn(ctx, "failed to load settings, using cached", "error", err)
	} else {
		next.settings = p.mergeSettings(ctx, raw)
	}
	// после настроек: тексты проверяются по своим переменным этого же снимка
	overrides, err := p.src.TextOverrides(ctx)
	if err != nil {
		stlog.Warn(ctx, "failed to load text overrides, using cached", "error", err)
	} else {
		next.texts = p.validTexts(ctx, overrides, next.settings.Vars)
	}
	p.snap.Store(&next)
}

func (p *Provider) mergeSettings(ctx context.Context, raw map[string][]byte) settings.Settings {
	s, err := settings.Merge(p.defaults, raw)
	if err != nil {
		stlog.Warn(ctx, "broken settings keys, using defaults for them", "error", err)
	}
	if err := s.Validate(); err != nil {
		stlog.Warn(ctx, "invalid settings, using env defaults", "error", err)
		return p.defaults
	}
	return s
}

// validTexts отбрасывает невалидные переопределения; warn — один раз на пару (key, тело).
func (p *Provider) validTexts(ctx context.Context, overrides, custom map[string]string) map[string]string {
	out := make(map[string]string, len(overrides))
	warned := map[string]string{}
	for key, body := range overrides {
		def, ok := texts.Lookup(key)
		if !ok {
			continue
		}
		if err := texts.ValidateStored(def, body, custom); err != nil {
			if prev, ok := p.warned[key]; !ok || prev != body {
				stlog.Warn(ctx, "invalid text override, using default", "key", key, "error", err)
			}
			warned[key] = body
			continue
		}
		out[key] = body
	}
	p.warned = warned
	return out
}
