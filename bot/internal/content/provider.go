// Package content — тексты и настройки бота из БД (правки из админки) с кешем на ttl.
package content

import (
	"context"
	"sync"
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

type Provider struct {
	src      Source
	defaults settings.Settings
	ttl      time.Duration
	now      func() time.Time

	mu       sync.Mutex
	loadedAt time.Time
	texts    map[string]string // только прошедшие texts.Validate
	settings settings.Settings
}

func New(src Source, defaults settings.Settings, ttl time.Duration, now func() time.Time) *Provider {
	if now == nil {
		now = time.Now
	}
	return &Provider{src: src, defaults: defaults, ttl: ttl, now: now, settings: defaults}
}

// Text — переопределение из БД или текст по умолчанию, отрендеренный texts.RenderDef.
func (p *Provider) Text(ctx context.Context, key string, vars map[string]string) string {
	def, ok := texts.Lookup(key)
	if !ok {
		stlog.Error(ctx, "unknown text key", "key", key)
		return key
	}
	body := def.Default
	p.mu.Lock()
	p.refresh(ctx)
	if b, ok := p.texts[key]; ok {
		body = b
	}
	p.mu.Unlock()
	return texts.RenderDef(def, body, vars)
}

// Button — подпись кнопки без переменных.
func (p *Provider) Button(ctx context.Context, key string) string {
	return p.Text(ctx, key, nil)
}

func (p *Provider) Settings(ctx context.Context) settings.Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refresh(ctx)
	return p.settings
}

// refresh перечитывает БД, если кеш старше ttl; вызывается под p.mu.
// Ошибка БД — остаются последние значения, следующая попытка через ttl.
// ponytail: чтение БД под мьютексом — параллельные апдейты ждут один запрос раз в ttl; singleflight, если станет узко.
func (p *Provider) refresh(ctx context.Context) {
	now := p.now()
	if !p.loadedAt.IsZero() && now.Sub(p.loadedAt) < p.ttl {
		return
	}
	p.loadedAt = now
	overrides, err := p.src.TextOverrides(ctx)
	if err != nil {
		stlog.Warn(ctx, "failed to load text overrides, using cached", "error", err)
	} else {
		p.texts = validTexts(ctx, overrides)
	}
	raw, err := p.src.SettingsRaw(ctx)
	if err != nil {
		stlog.Warn(ctx, "failed to load settings, using cached", "error", err)
		return
	}
	s, err := settings.Merge(p.defaults, raw)
	if err != nil {
		stlog.Warn(ctx, "broken settings keys, using defaults for them", "error", err)
	}
	if err := s.Validate(); err != nil {
		stlog.Warn(ctx, "invalid settings, using env defaults", "error", err)
		s = p.defaults
	}
	p.settings = s
}

func validTexts(ctx context.Context, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(overrides))
	for key, body := range overrides {
		def, ok := texts.Lookup(key)
		if !ok {
			continue
		}
		if err := texts.Validate(def, body); err != nil {
			stlog.Warn(ctx, "invalid text override, using default", "key", key, "error", err)
			continue
		}
		out[key] = body
	}
	return out
}
