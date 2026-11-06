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
	"unicode/utf16"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

type fakeSource struct {
	mu    sync.Mutex
	texts map[string]string
	raw   map[string][]byte
	err   error
	calls int
	// block != nil — TextOverrides сигналит в started и висит до закрытия block или отмены ctx
	block   chan struct{}
	started chan struct{}

	contacts     map[int64]model.UserContact
	contactErr   error
	contactCalls int
	styles       map[string]string
}

func (f *fakeSource) TextOverrides(ctx context.Context) (map[string]string, error) {
	f.mu.Lock()
	f.calls++
	block, started, texts, err := f.block, f.started, f.texts, f.err
	f.mu.Unlock()
	if block != nil {
		if started != nil {
			started <- struct{}{}
		}
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return texts, err
}

func (f *fakeSource) SettingsRaw(context.Context) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.raw, f.err
}

func (f *fakeSource) UserContact(_ context.Context, id int64) (model.UserContact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contactCalls++
	return f.contacts[id], f.contactErr
}

func (f *fakeSource) TextStyles(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.styles, f.err
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

// Зависшая БД: Text не ждёт дольше таймаута обновления и отдаёт значение по умолчанию.
func TestHungSourceTimesOut(t *testing.T) {
	p, _, ctx, buf := setup(&fakeSource{block: make(chan struct{})})
	content.SetRefreshTimeout(p, 50*time.Millisecond)
	start := time.Now()
	if got := p.Button(ctx, "button.pay"); got != "Оплатить" {
		t.Fatalf("button = %q", got)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Text ждал %v", d)
	}
	if !strings.Contains(buf.String(), "deadline exceeded") {
		t.Fatalf("нет warn: %s", buf.String())
	}
}

// Пока одна горутина обновляет кеш, остальные сразу получают прежний снимок.
func TestConcurrentReadersDoNotWaitForRefresh(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"button.pay": "Платить"}}
	p, c, ctx, _ := setup(src)
	p.Button(ctx, "button.pay")

	release, started := make(chan struct{}), make(chan struct{}, 1)
	src.set(func(f *fakeSource) {
		f.block, f.started = release, started
		f.texts = map[string]string{"button.pay": "Оплата"}
	})
	c.t = c.t.Add(time.Minute)
	done := make(chan string)
	go func() { done <- p.Button(ctx, "button.pay") }()
	<-started

	got := make(chan string)
	go func() { got <- p.Button(ctx, "button.pay") }()
	select {
	case v := <-got:
		if v != "Платить" {
			t.Fatalf("во время обновления = %q", v)
		}
	case <-time.After(time.Second):
		t.Fatal("читатель ждёт обновления")
	}
	close(release)
	if v := <-done; v != "Оплата" {
		t.Fatalf("обновивший = %q", v)
	}
	if v := p.Button(ctx, "button.pay"); v != "Оплата" {
		t.Fatalf("после обновления = %q", v)
	}
}

// Предупреждение о невалидном переопределении — один раз, пока тело не изменится.
func TestInvalidOverrideWarnsOnce(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"menu.greeting": "<b>битая"}}
	p, c, ctx, buf := setup(src)
	for range 3 {
		p.Text(ctx, "menu.greeting", nil)
		c.t = c.t.Add(time.Minute)
	}
	if n := strings.Count(buf.String(), "invalid text override"); n != 1 {
		t.Fatalf("warn %d раз, want 1", n)
	}
	src.set(func(f *fakeSource) { f.texts = map[string]string{"menu.greeting": "<i>битая"} })
	p.Text(ctx, "menu.greeting", nil)
	if n := strings.Count(buf.String(), "invalid text override"); n != 2 {
		t.Fatalf("warn после правки %d раз, want 2", n)
	}
}

func TestGlobalsFromSettings(t *testing.T) {
	src := &fakeSource{
		texts: map[string]string{"error.generic": "{{.phone}} {{.bank}} {{.supportUrl}} {{.my}}"},
		raw:   map[string][]byte{"vars": []byte(`{"my":"1<2"}`)},
	}
	p, _, ctx, _ := setup(src)
	if got, want := p.Text(ctx, "error.generic", nil), "+7 800 Т-Банк https://t.me/support 1&lt;2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUserGlobals(t *testing.T) {
	exp := time.Date(2026, 11, 2, 12, 4, 0, 0, time.UTC)
	src := &fakeSource{
		texts:    map[string]string{"error.generic": "{{.name}}|{{.username}}|{{.telegramId}}|{{.expireAt}}"},
		contacts: map[int64]model.UserContact{7: {FirstName: "<Ann>", Username: "ann", ExpireAt: &exp}},
	}
	p, _, ctx, _ := setup(src)
	for name, tc := range map[string]struct {
		ctx  context.Context
		want string
	}{
		"no user":     {ctx, "друг|—|—|—"},
		"from update": {content.WithUser(ctx, 7, "Upd", ""), "Upd|—|7|2 ноября 2026 г. в 15:04, MSK"},
		"by id":       {content.ForUser(ctx, 7), "&lt;Ann&gt;|ann|7|2 ноября 2026 г. в 15:04, MSK"},
		"not in db":   {content.ForUser(ctx, 9), "друг|—|9|—"},
	} {
		if got := p.Text(tc.ctx, "error.generic", nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

// БД — только когда текст просит то, чего нет в контексте.
func TestUserContactLoadedOnlyWhenAsked(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"error.generic": "{{.name}}", "payment.tariffs": "{{.expireAt}}"}}
	p, _, ctx, _ := setup(src)
	p.Text(ctx, "payment.confirmed", nil)
	p.Text(content.ForUser(ctx, 7), "payment.confirmed", nil) // нет переменных
	p.Text(content.WithUser(ctx, 7, "A", ""), "error.generic", nil)
	p.Text(content.WithUser(ctx, 7, "A", ""), "payment.tariffs", map[string]string{"expireAt": "x"})
	if src.contactCalls != 0 {
		t.Fatalf("calls = %d", src.contactCalls)
	}
	p.Text(content.WithUser(ctx, 7, "A", ""), "payment.tariffs", nil)
	if src.contactCalls != 1 {
		t.Fatalf("calls = %d", src.contactCalls)
	}
}

func TestUserContactErrorDoesNotBreakText(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"error.generic": "<b>{{.name}}</b> {{.expireAt}}"}, contactErr: errors.New("db down")}
	p, _, ctx, _ := setup(src)
	if got := p.Text(content.ForUser(ctx, 7), "error.generic", nil); got != "<b>друг</b> —" {
		t.Fatalf("got %q", got)
	}
}

// Удалённая своя переменная не отменяет правку и не ломает рендер; ничего не осталось — текст по умолчанию.
func TestUnknownVarRendersEmpty(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"error.generic": "<b>{{.gone}}</b>", "payment.tariffs": "Выбор {{.gone}}", "button.pay": "{{.gone}}"}}
	p, _, ctx, _ := setup(src)
	if got := p.Text(ctx, "payment.tariffs", nil); got != "Выбор " {
		t.Fatalf("got %q", got)
	}
	if got := p.Text(ctx, "error.generic", nil); got != "К сожалению, произошла ошибка, обратитесь в поддержку" {
		t.Fatalf("blank message: %q", got)
	}
	if got := p.Button(ctx, "button.pay"); got != "Оплатить" {
		t.Fatalf("blank button: %q", got)
	}
}

// Гонка снимка: тело и значения своих переменных приходят из одного поколения, данные не портятся (go test -race).
func TestSnapshotConsistentUnderRefresh(t *testing.T) {
	src := &fakeSource{}
	gen := func(n string) {
		src.set(func(f *fakeSource) {
			f.texts = map[string]string{"error.generic": n + "={{.v" + n + "}}"}
			f.raw = map[string][]byte{"vars": []byte(`{"v` + n + `":"` + n + `"}`)}
		})
	}
	gen("a")
	p := content.New(src, defaults, 0, nil) // ttl 0: каждый вызов обновляет
	ctx := t.Context()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				gen(map[bool]string{true: "a", false: "b"}[i%2 == 0])
			}
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 200 {
				got := p.Text(ctx, "error.generic", nil)
				// поколения могут разойтись только в «пусто», чужого значения быть не может
				if got == "a=b" || got == "b=a" {
					t.Errorf("got %q", got)
				}
				_ = p.Settings(ctx).TextVars()
			}
		})
	}
	time.Sleep(20 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestStyle(t *testing.T) {
	src := &fakeSource{styles: map[string]string{"button.pay": "danger", "button.buy": "", "button.info": "мусор"}}
	p, _, ctx, _ := setup(src)
	for key, want := range map[string]keyboard.Style{
		"button.pay":   keyboard.StyleDanger,
		"button.buy":   keyboard.StyleDefault, // «Обычная» сохранена явно: важнее стиля из кода
		"button.info":  keyboard.StyleSuccess, // чужое значение игнорируется
		"button.renew": keyboard.StyleSuccess, // не сохраняли
	} {
		if got := p.Style(ctx, key, keyboard.StyleSuccess); got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

// Подпись, которую Telegram отверг бы (пустая или длиннее 64), заменяется текстом по умолчанию; warn один раз на ключ.
func TestButtonSafeGuard(t *testing.T) {
	src := &fakeSource{texts: map[string]string{"button.pay": "{{.name}}"}}
	p, _, ctx, buf := setup(src)
	if got := p.Text(ctx, "button.pay", map[string]string{"name": strings.Repeat("я", 64)}); utf16Len(got) != 64 {
		t.Fatalf("64 символа должны пройти: %q", got)
	}
	for range 3 {
		if got := p.Text(ctx, "button.pay", map[string]string{"name": strings.Repeat("я", 65)}); got != "Оплатить" {
			t.Fatalf("65 символов: got %q", got)
		}
	}
	if got := p.Text(ctx, "button.pay", map[string]string{"name": "  "}); got != "Оплатить" {
		t.Fatalf("пусто: got %q", got)
	}
	if n := strings.Count(buf.String(), "does not fit"); n != 1 {
		t.Fatalf("warn %d раз, want 1: %s", n, buf)
	}
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }
