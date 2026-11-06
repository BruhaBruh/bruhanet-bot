package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// Identity — имена в Telegram: бот (getMe) и чат админа (getChat). Необязателен: нет — поля пустые.
type Identity interface {
	Me(ctx context.Context) (username, name string, err error)
	Chat(ctx context.Context, chatID int64) (username, name string, err error)
}

const (
	identityTimeout = 3 * time.Second
	identityRetry   = 30 * time.Second // после неудачи Telegram не дёргаем чаще (и публичный /auth/bot не раскручивает запросы)
)

// who — username (без @) и имя; пустые поля — не знаем.
type who struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// lazyWho держит удачный ответ навсегда (ник бота и админа почти не меняются), неудачный — не чаще раза в identityRetry.
type lazyWho struct {
	mu    sync.Mutex
	v     who
	ok    bool
	tried time.Time
}

func (l *lazyWho) get(ctx context.Context, now time.Time, fetch func(context.Context) (string, string, error)) who {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ok || (!l.tried.IsZero() && now.Sub(l.tried) < identityRetry) {
		return l.v
	}
	l.tried = now
	// Запрос клиента мог оборваться: ответ Telegram кэшируется для всех, а отмена не должна взводить окно сбоя.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), identityTimeout)
	defer cancel()
	username, name, err := fetch(ctx)
	if err != nil {
		stlog.Warn(ctx, "failed to load telegram identity", "error", err)
		return who{}
	}
	l.v, l.ok = who{strings.TrimPrefix(username, "@"), name}, true
	return l.v
}

func (s *server) botWho(ctx context.Context) who {
	if s.Identity == nil {
		return who{}
	}
	return s.botCache.get(ctx, s.Now(), s.Identity.Me)
}

// adminWho — из users (бот уже видел админа), иначе getChat.
func (s *server) adminWho(ctx context.Context) who {
	var w who
	switch u, err := s.Store.UserByID(ctx, s.AdminID); {
	case err == nil:
		w = who{u.Username, u.Name}
	case !errors.Is(err, store.ErrNotFound):
		stlog.Warn(ctx, "failed to load admin user", "error", err)
	}
	if (w.Username == "" && w.Name == "") && s.Identity != nil {
		return s.adminCache.get(ctx, s.Now(), func(ctx context.Context) (string, string, error) {
			return s.Identity.Chat(ctx, s.AdminID)
		})
	}
	return w
}

// me — сессия уже проверена requireSession, а она пускает только AdminID.
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	a := s.adminWho(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"telegramId": s.AdminID, "username": a.Username, "name": a.Name, "bot": s.botWho(r.Context()),
	})
}

// authBot — публично (для страницы входа, где сессии ещё нет): только ник и имя бота, ничего про админа.
func (s *server) authBot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.botWho(r.Context()))
}
