package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/api/internal/telegram"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

// AvatarSource — аватарки через Bot API (api/internal/telegram). Необязателен: нет — все ответы 404.
type AvatarSource interface {
	BotID(ctx context.Context) (int64, error)
	ProfilePhoto(ctx context.Context, userID int64) (fileID string, err error)
	OpenFile(ctx context.Context, fileID string) (io.ReadCloser, error)
}

const (
	maxAvatarSize     = 1 << 20
	avatarFetchTO     = 10 * time.Second
	avatarTTL         = 6 * time.Hour
	avatarMissTTL     = 30 * time.Minute // у юзера нет фото
	avatarErrTTL      = time.Minute      // сеть, 5xx, мусор: скоро пробуем снова
	avatarMaxRetry    = 5 * time.Minute  // потолок паузы по retry_after
	avatarCacheCap    = 500
	avatarConcurrency = 4
	avatarBotKey      = 0 // telegram_id юзера не бывает 0
)

// avatar — запись кэша; data == nil — «фото нет», отдаём 404 до until.
type avatar struct {
	data  []byte
	ct    string
	etag  string
	until time.Time
}

type avatarCall struct {
	done chan struct{}
	res  *avatar
}

// avatarCache — память процесса: запись на юзера, один живой запрос к Telegram на ключ, не больше
// avatarConcurrency запросов к Telegram разом, вытеснение самой старой записи.
type avatarCache struct {
	mu     sync.Mutex
	m      map[int64]*avatar
	order  []int64
	flight map[int64]*avatarCall
	sem    chan struct{}
}

func newAvatarCache() *avatarCache {
	return &avatarCache{m: map[int64]*avatar{}, flight: map[int64]*avatarCall{}, sem: make(chan struct{}, avatarConcurrency)}
}

func (c *avatarCache) fresh(key int64, now time.Time) *avatar {
	if a := c.m[key]; a != nil && now.Before(a.until) {
		return a
	}
	return nil
}

func (c *avatarCache) put(key int64, a *avatar) {
	if _, ok := c.m[key]; !ok {
		c.order = append(c.order, key)
		if len(c.order) > avatarCacheCap {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.m[key] = a
}

// get отдаёт запись из кэша либо делает (одним запросом на всех ждущих) fetch.
func (c *avatarCache) get(ctx context.Context, key int64, now time.Time, fetch func(context.Context) *avatar) *avatar {
	c.mu.Lock()
	if a := c.fresh(key, now); a != nil {
		c.mu.Unlock()
		return a
	}
	call, ok := c.flight[key]
	if !ok {
		call = &avatarCall{done: make(chan struct{})}
		c.flight[key] = call
		go func() {
			// запрос клиента мог оборваться: результат нужен остальным ждущим и кэшу
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), avatarFetchTO)
			defer cancel()
			var a *avatar
			select {
			case c.sem <- struct{}{}:
				a = fetch(fctx)
				<-c.sem
			case <-fctx.Done():
				a = &avatar{}
			}
			c.mu.Lock()
			if a.until.IsZero() { // fetch отдаёт ttl относительно now
				a.until = now.Add(avatarErrTTL)
			}
			c.put(key, a)
			delete(c.flight, key)
			call.res = a
			c.mu.Unlock()
			close(call.done)
		}()
	}
	c.mu.Unlock()
	select {
	case <-call.done:
		return call.res
	case <-ctx.Done():
		return nil
	}
}

// fetchAvatar ходит в Telegram и всегда возвращает запись: с данными или отрицательную со своим сроком.
func (s *server) fetchAvatar(ctx context.Context, key int64, now time.Time) *avatar {
	miss := func(ttl time.Duration) *avatar { return &avatar{until: now.Add(ttl)} }
	fail := func(err error) *avatar {
		if tm, ok := errors.AsType[*bot.TooManyRequestsError](err); ok {
			return miss(min(max(time.Duration(tm.RetryAfter)*time.Second, time.Second), avatarMaxRetry))
		}
		switch {
		case errors.Is(err, bot.ErrorBadRequest), errors.Is(err, bot.ErrorForbidden), errors.Is(err, telegram.ErrNoPhoto):
			return miss(avatarMissTTL)
		}
		stlog.Warn(ctx, "failed to fetch avatar", "error", err)
		return miss(avatarErrTTL)
	}
	id := key
	if key == avatarBotKey {
		var err error
		if id, err = s.Avatars.BotID(ctx); err != nil {
			return fail(err)
		}
	}
	fileID, err := s.Avatars.ProfilePhoto(ctx, id)
	if err != nil {
		return fail(err)
	}
	file, err := s.Avatars.OpenFile(ctx, fileID)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarSize+1))
	if err != nil {
		return fail(err)
	}
	if len(data) > maxAvatarSize {
		return fail(errors.New("avatar is too large"))
	}
	ct := http.DetectContentType(data)
	if !slices.Contains(photoTypes, ct) {
		return fail(errors.New("avatar is not an image"))
	}
	sum := sha256.Sum256(data)
	return &avatar{data: data, ct: ct, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, until: now.Add(avatarTTL)}
}

// avatarImage отдаёт аватарку юзера из users (или бота — /api/avatars/bot) потоком через Bot API. Чужие id не
// ищем: нет в users — 404, как и при любом другом сбое (причина наружу не выходит).
func (s *server) avatarImage(w http.ResponseWriter, r *http.Request) {
	notFound := func() { writeError(w, http.StatusNotFound, "not_found") }
	var key int64
	if raw := r.PathValue("id"); raw != "bot" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 || s.Avatars == nil {
			notFound()
			return
		}
		key = id
	} else if s.Avatars == nil {
		notFound()
		return
	}
	now := s.Now()
	s.avatars.mu.Lock()
	cached := s.avatars.fresh(key, now) != nil
	s.avatars.mu.Unlock()
	if !cached && key != avatarBotKey {
		switch _, err := s.Store.UserByID(r.Context(), key); {
		case errors.Is(err, store.ErrNotFound):
			notFound()
			return
		case err != nil:
			internal(w, r, "avatar", err)
			return
		}
	}
	a := s.avatars.get(r.Context(), key, now, func(ctx context.Context) *avatar { return s.fetchAvatar(ctx, key, now) })
	if a == nil || a.data == nil {
		notFound()
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "private, max-age=3600")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("ETag", a.etag)
	if strings.Contains(r.Header.Get("If-None-Match"), a.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", a.ct)
	h.Set("Content-Length", strconv.Itoa(len(a.data)))
	_, _ = w.Write(a.data)
}
