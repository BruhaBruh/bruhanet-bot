package content_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

type fakeSource struct {
	mu    sync.Mutex
	texts map[string]string
	raw   map[string][]byte
	err   error
	calls int
}

func (f *fakeSource) TextOverrides(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.texts, f.err
}

func (f *fakeSource) SettingsRaw(context.Context) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.raw, f.err
}

func (f *fakeSource) set(fn func(f *fakeSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

var defaults = settings.Defaults(map[int]int{1: 150}, "+7 800", "Т-Банк", "https://t.me/support")

func setup(src *fakeSource) (*content.Provider, *clock, context.Context, *bytes.Buffer) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	var buf bytes.Buffer
	ctx := stlog.Wrap(context.Background(), slog.New(slog.NewTextHandler(&buf, nil)))
	return content.New(src, defaults, 10*time.Second, c.now), c, ctx, &buf
}

func TestDefaultsAndEscaping(t *testing.T) {
	p, _, ctx, _ := setup(&fakeSource{})
	if got := p.Text(ctx, "menu.greeting", map[string]string{"name": "a<b>&"}); got != "Здравствуйте, <b>a&lt;b&gt;&amp;</b>" {
		t.Fatalf("greeting = %q", got)
	}
	if got := p.Text(ctx, "button.tariff", map[string]string{"months": "3 месяца", "price": "500"}); got != "3 месяца за 500 ₽" {
		t.Fatalf("tariff = %q", got)
	}
	if got := p.Settings(ctx); got.Phone != "+7 800" || got.Prices[1] != 150 {
		t.Fatalf("settings = %+v", got)
	}
}

func TestOverrideAndTTL(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"button.pay": "Платить"}}
	p, c, ctx, _ := setup(src)
	if got := p.Button(ctx, "button.pay"); got != "Платить" {
		t.Fatalf("button = %q", got)
	}
	src.set(func(f *fakeSource) {
		f.texts = map[string]string{"button.pay": "Оплата"}
		f.raw = map[string][]byte{"phone": []byte(`"+7 999"`)}
	})
	c.t = c.t.Add(9 * time.Second)
	if got := p.Button(ctx, "button.pay"); got != "Платить" {
		t.Fatalf("до TTL кеш не должен обновиться: %q", got)
	}
	c.t = c.t.Add(2 * time.Second)
	if got := p.Button(ctx, "button.pay"); got != "Оплата" {
		t.Fatalf("после TTL = %q", got)
	}
	if got := p.Settings(ctx).Phone; got != "+7 999" {
		t.Fatalf("phone = %q", got)
	}
	if src.calls != 2 {
		t.Fatalf("calls = %d, want 2", src.calls)
	}
}

// Review Focus 3: БД недоступна — работаем на последних значениях, ошибка в лог.
func TestSourceErrorKeepsLast(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"button.pay": "Платить"}}
	p, c, ctx, buf := setup(src)
	p.Button(ctx, "button.pay")
	src.set(func(f *fakeSource) { f.err = errors.New("db down") })
	c.t = c.t.Add(time.Minute)
	if got := p.Button(ctx, "button.pay"); got != "Платить" {
		t.Fatalf("после ошибки = %q", got)
	}
	if !strings.Contains(buf.String(), "db down") {
		t.Fatalf("нет warn в логе: %s", buf.String())
	}

	// БД недоступна с самого старта — значения по умолчанию
	p2, _, ctx2, _ := setup(&fakeSource{err: errors.New("db down")})
	if got := p2.Button(ctx2, "button.pay"); got != "Оплатить" {
		t.Fatalf("default = %q", got)
	}
	if got := p2.Settings(ctx2).Bank; got != "Т-Банк" {
		t.Fatalf("bank = %q", got)
	}
}

func TestInvalidOverrideFallsBack(t *testing.T) {
	src := &fakeSource{
		texts: map[string]string{"menu.greeting": "<b>битая", "button.pay": "<b>x</b>"},
		raw:   map[string][]byte{"prices": []byte(`{"1":0}`), "bank": []byte(`"Сбер"`)},
	}
	p, _, ctx, buf := setup(src)
	if got := p.Text(ctx, "menu.greeting", map[string]string{"name": "x"}); got != "Здравствуйте, <b>x</b>" {
		t.Fatalf("greeting = %q", got)
	}
	if got := p.Button(ctx, "button.pay"); got != "Оплатить" {
		t.Fatalf("button = %q", got)
	}
	// цена 0 — настройки целиком невалидны, берутся из env
	if got := p.Settings(ctx); got.Bank != "Т-Банк" || got.Prices[1] != 150 {
		t.Fatalf("settings = %+v", got)
	}
	if !strings.Contains(buf.String(), "invalid text override") || !strings.Contains(buf.String(), "invalid settings") {
		t.Fatalf("нет warn: %s", buf.String())
	}
}

func TestBrokenSettingKeySkipped(t *testing.T) {
	src := &fakeSource{raw: map[string][]byte{"bank": []byte(`"Сбер"`), "phone": []byte(`{`)}}
	p, _, ctx, buf := setup(src)
	if got := p.Settings(ctx); got.Bank != "Сбер" || got.Phone != "+7 800" {
		t.Fatalf("settings = %+v", got)
	}
	if !strings.Contains(buf.String(), "broken settings") {
		t.Fatalf("нет warn: %s", buf.String())
	}
}
