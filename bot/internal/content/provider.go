// Package content — тексты и настройки бота из БД (правки из админки) с кешем на ttl.
package content

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// Source — store.Store.
type Source interface {
	TextOverrides(ctx context.Context) (map[string]string, error)
	SettingsRaw(ctx context.Context) (map[string][]byte, error)
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
	body := def.Default
	if b, ok := p.current(ctx).texts[key]; ok {
		body = b
	}
	return texts.RenderDef(def, body, vars)
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
	overrides, err := p.src.TextOverrides(ctx)
	if err != nil {
		stlog.Warn(ctx, "failed to load text overrides, using cached", "error", err)
	} else {
		next.texts = p.validTexts(ctx, overrides)
	}
	if raw, err := p.src.SettingsRaw(ctx); err != nil {
		stlog.Warn(ctx, "failed to load settings, using cached", "error", err)
	} else {
		next.settings = p.mergeSettings(ctx, raw)
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
func (p *Provider) validTexts(ctx context.Context, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(overrides))
	warned := map[string]string{}
	for key, body := range overrides {
		def, ok := texts.Lookup(key)
		if !ok {
			continue
		}
		if err := texts.Validate(def, body); err != nil {
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
