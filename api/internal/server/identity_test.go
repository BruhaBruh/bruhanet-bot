package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
)

type fakeIdentity struct {
	mu                 sync.Mutex
	meCalls, chatCalls int
	meErr, chatErr     error
	botUser, botName   string
	chatUser, chatName string
}

func (f *fakeIdentity) Me(context.Context) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.meCalls++
	return f.botUser, f.botName, f.meErr
}

func (f *fakeIdentity) Chat(_ context.Context, id int64) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chatCalls++
	if id != adminID {
		return "", "", errors.New("wrong chat")
	}
	return f.chatUser, f.chatName, f.chatErr
}

// identityServer — сервер с управляемыми часами и Identity.
func identityServer(t *testing.T, id server.Identity, clock *time.Time) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	h := server.New(server.Deps{
		Store: st, Panel: &fakePanel{}, Telegram: &fakeTG{}, Identity: id,
		Defaults: settings.Defaults(map[int]int{1: 150}, "+7 900", "Т-Банк", "https://t.me/support"),
		AdminID:  adminID, BaseURL: "http://localhost:5173",
		Now:    func() time.Time { return *clock },
		Static: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}},
	})
	return h, st
}

type meJSON struct {
	TelegramID int64  `json:"telegramId"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Bot        struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"bot"`
}

func TestMeIdentity(t *testing.T) {
	clock := now
	id := &fakeIdentity{botUser: "bruhanet_payment_bot", botName: "BruhaNet Payment", chatUser: "@fromchat", chatName: "Из чата"}
	h, st := identityServer(t, id, &clock)
	c := st.addSession(t, adminID, now.Add(48*time.Hour))

	// админа бот ещё не видел: getChat; в ответе username без @
	var me meJSON
	getJSON(t, h, "/api/me", c, &me)
	if me.TelegramID != adminID || me.Username != "fromchat" || me.Name != "Из чата" || me.Bot.Username != "bruhanet_payment_bot" || me.Bot.Name != "BruhaNet Payment" {
		t.Fatalf("me = %+v", me)
	}
	// удачные ответы кэшируются
	getJSON(t, h, "/api/me", c, &me)
	if id.meCalls != 1 || id.chatCalls != 1 {
		t.Fatalf("calls me=%d chat=%d", id.meCalls, id.chatCalls)
	}

	// админ появился в users — берём оттуда (при пустом кэше getChat не нужен)
	h2, st2 := identityServer(t, &fakeIdentity{}, &clock)
	st2.users = []model.UserSummary{{TelegramID: adminID, Username: "bruhabruh4", Name: "Bruha Bruh"}}
	getJSON(t, h2, "/api/me", st2.addSession(t, adminID, now.Add(48*time.Hour)), &me)
	if me.Username != "bruhabruh4" || me.Name != "Bruha Bruh" {
		t.Fatalf("from users: %+v", me)
	}
}

// Telegram недоступен: поля пустые, не 500; повтор не раньше чем через 30 с, потом снова пробуем.
func TestMeIdentityFailureNeverFails(t *testing.T) {
	clock := now
	id := &fakeIdentity{meErr: errors.New("down"), chatErr: errors.New("down")}
	h, st := identityServer(t, id, &clock)
	c := st.addSession(t, adminID, now.Add(48*time.Hour))
	var me meJSON
	getJSON(t, h, "/api/me", c, &me)
	if me.Username != "" || me.Name != "" || me.Bot.Username != "" || me.TelegramID != adminID {
		t.Fatalf("me = %+v", me)
	}
	getJSON(t, h, "/api/me", c, &me)
	if id.meCalls != 1 || id.chatCalls != 1 {
		t.Fatalf("retried too early: me=%d chat=%d", id.meCalls, id.chatCalls)
	}
	clock = clock.Add(31 * time.Second)
	id.meErr, id.botUser = nil, "bot"
	getJSON(t, h, "/api/me", c, &me)
	if me.Bot.Username != "bot" || id.meCalls != 2 {
		t.Fatalf("no recovery: %+v calls=%d", me, id.meCalls)
	}
}

// /auth/bot открыт без сессии и отдаёт только ник и имя бота: ни данных админа, ни id.
func TestAuthBotIsPublicAndMinimal(t *testing.T) {
	clock := now
	id := &fakeIdentity{botUser: "mybot", botName: "My Bot", chatUser: "secretadmin", chatName: "Secret Admin"}
	h, st := identityServer(t, id, &clock)
	st.users = []model.UserSummary{{TelegramID: adminID, Username: "secretadmin", Name: "Secret Admin"}}

	w := do(h, http.MethodGet, "/auth/bot", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["username"] != "mybot" || got["name"] != "My Bot" {
		t.Fatalf("body = %s", w.Body)
	}
	for _, leak := range []string{"secretadmin", "Secret Admin", `"telegramId"`} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("leaks %q: %s", leak, w.Body)
		}
	}
	if id.chatCalls != 0 {
		t.Fatal("public endpoint touched the admin chat")
	}
	// Telegram недоступен — всё равно 200 с пустыми полями
	h2, _ := identityServer(t, &fakeIdentity{meErr: errors.New("down")}, &clock)
	if w := do(h2, http.MethodGet, "/auth/bot", "", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"username":""`) {
		t.Fatalf("down: %d %s", w.Code, w.Body)
	}
	// без Identity вообще
	h3, _ := newServer(t, "http://localhost:5173")
	if w := do(h3, http.MethodGet, "/auth/bot", "", ""); w.Code != http.StatusOK {
		t.Fatalf("nil identity: %d", w.Code)
	}
}

// ctxIdentity падает, если ему передали отменённый контекст.
type ctxIdentity struct{ fakeIdentity }

func (f *ctxIdentity) Me(ctx context.Context) (string, string, error) {
	if err := ctx.Err(); err != nil {
		f.mu.Lock()
		f.meCalls++
		f.mu.Unlock()
		return "", "", err
	}
	return f.fakeIdentity.Me(ctx)
}

// Клиент оборвал запрос: Telegram всё равно опрашивается живым контекстом, окно сбоя не взводится.
func TestIdentityIgnoresClientCancel(t *testing.T) {
	clock := now
	id := &ctxIdentity{fakeIdentity{botUser: "mybot", botName: "My Bot"}}
	h, _ := identityServer(t, id, &clock)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/auth/bot", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	var got struct{ Username string }
	getJSON(t, h, "/auth/bot", "", &got)
	if got.Username != "mybot" {
		t.Fatalf("cancelled client armed the failure window: %+v", got)
	}
}
