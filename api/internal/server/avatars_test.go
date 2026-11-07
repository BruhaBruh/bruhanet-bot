package server_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/api/internal/telegram"
	"github.com/bruhabruh/bruhanet/core/model"
)

// fakeAvatars — Telegram для аватарок: фото по id, счётчик обращений и пиковая параллельность.
type fakeAvatars struct {
	mu        sync.Mutex
	photos    map[int64][]byte // нет ключа — у юзера нет фото
	err       error            // ошибка ProfilePhoto
	botID     int64
	botErr    error
	calls     atomic.Int32
	cur, peak int32
	delay     time.Duration
}

func (f *fakeAvatars) BotID(context.Context) (int64, error) { return f.botID, f.botErr }

func (f *fakeAvatars) ProfilePhoto(_ context.Context, id int64) (string, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.cur++
	f.peak = max(f.peak, f.cur)
	f.mu.Unlock()
	time.Sleep(f.delay)
	f.mu.Lock()
	f.cur--
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	if _, ok := f.photos[id]; !ok {
		return "", telegram.ErrNoPhoto
	}
	return "file-" + strconv.FormatInt(id, 10), nil
}

func (f *fakeAvatars) OpenFile(_ context.Context, fileID string) (io.ReadCloser, error) {
	id, _ := strconv.ParseInt(strings.TrimPrefix(fileID, "file-"), 10, 64)
	return io.NopCloser(bytes.NewReader(f.photos[id])), nil
}

var avatarPNG = append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{7}, 100)...)

func avatarServer(t *testing.T, tg server.AvatarSource, clock *time.Time, ids ...int64) (http.Handler, string) {
	t.Helper()
	st := newFakeStore()
	for _, id := range ids {
		st.users = append(st.users, model.UserSummary{TelegramID: id})
	}
	h := server.New(server.Deps{
		Store: st, Panel: &fakePanel{}, Telegram: &fakeTG{}, Avatars: tg, AdminID: adminID,
		BaseURL: "http://localhost:5173", Now: func() time.Time { return *clock },
		Static: fstest.MapFS{"index.html": {Data: []byte(indexHTML)}},
	})
	return h, st.addSession(t, adminID, now.Add(24*time.Hour))
}

func getAvatar(h http.Handler, cookie, id string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/avatars/"+id, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAvatarServed(t *testing.T) {
	clock := now
	tg := &fakeAvatars{photos: map[int64][]byte{42: avatarPNG}}
	h, c := avatarServer(t, tg, &clock, 42)

	w := getAvatar(h, c, "42")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), avatarPNG) || w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no etag")
	}
	if w2 := getAvatar(h, c, "42", "If-None-Match", etag); w2.Code != http.StatusNotModified || w2.Body.Len() != 0 {
		t.Fatalf("304: %d", w2.Code)
	}
	if tg.calls.Load() != 1 {
		t.Fatalf("telegram calls = %d, want cached", tg.calls.Load())
	}
	clock = clock.Add(6*time.Hour + time.Second)
	getAvatar(h, c, "42")
	if tg.calls.Load() != 2 {
		t.Fatalf("after ttl calls = %d", tg.calls.Load())
	}
}

func TestAvatarUnknownAndAuth(t *testing.T) {
	clock := now
	tg := &fakeAvatars{photos: map[int64][]byte{42: avatarPNG, 99: avatarPNG}}
	h, c := avatarServer(t, tg, &clock, 42)

	if w := getAvatar(h, "", "42"); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	for _, id := range []string{"99", "abc", "-5", "0"} {
		if w := getAvatar(h, c, id); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", id, w.Code)
		}
	}
	if tg.calls.Load() != 0 {
		t.Fatal("unknown id reached telegram")
	}
	h2, c2 := avatarServer(t, nil, &clock, 42)
	if w := getAvatar(h2, c2, "42"); w.Code != http.StatusNotFound {
		t.Fatalf("no source: %d", w.Code)
	}
}

func TestAvatarNegativeCache(t *testing.T) {
	clock := now
	tg := &fakeAvatars{}
	h, c := avatarServer(t, tg, &clock, 42)
	for range 3 {
		if w := getAvatar(h, c, "42"); w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != `{"error":"not_found"}` {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	if tg.calls.Load() != 1 {
		t.Fatalf("calls = %d", tg.calls.Load())
	}
	clock = clock.Add(31 * time.Minute)
	tg.photos = map[int64][]byte{42: avatarPNG}
	if w := getAvatar(h, c, "42"); w.Code != 200 {
		t.Fatalf("after miss ttl: %d", w.Code)
	}
}

func TestAvatarTransientErrorsRetrySoon(t *testing.T) {
	clock := now
	tg := &fakeAvatars{err: errors.New("network down"), photos: map[int64][]byte{42: avatarPNG}}
	h, c := avatarServer(t, tg, &clock, 42)
	if w := getAvatar(h, c, "42"); w.Code != http.StatusNotFound {
		t.Fatalf("%d", w.Code)
	}
	getAvatar(h, c, "42")
	if tg.calls.Load() != 1 {
		t.Fatalf("calls = %d", tg.calls.Load())
	}
	clock = clock.Add(61 * time.Second)
	tg.err = nil
	if w := getAvatar(h, c, "42"); w.Code != 200 {
		t.Fatalf("after 1 min: %d", w.Code)
	}
}

func TestAvatarTooManyRequests(t *testing.T) {
	clock := now
	tg := &fakeAvatars{err: &bot.TooManyRequestsError{Message: "slow", RetryAfter: 120}, photos: map[int64][]byte{42: avatarPNG}}
	h, c := avatarServer(t, tg, &clock, 42)
	if w := getAvatar(h, c, "42"); w.Code != http.StatusNotFound {
		t.Fatalf("%d", w.Code)
	}
	clock = clock.Add(100 * time.Second)
	tg.err = nil
	if w := getAvatar(h, c, "42"); w.Code != http.StatusNotFound || tg.calls.Load() != 1 {
		t.Fatalf("within retry_after: %d calls %d", w.Code, tg.calls.Load())
	}
	clock = clock.Add(21 * time.Second)
	if w := getAvatar(h, c, "42"); w.Code != 200 {
		t.Fatalf("after retry_after: %d", w.Code)
	}
	// retry_after больше потолка — не дольше 5 минут
	tg2 := &fakeAvatars{err: &bot.TooManyRequestsError{RetryAfter: 3600}, photos: map[int64][]byte{42: avatarPNG}}
	h2, c2 := avatarServer(t, tg2, &clock, 42)
	getAvatar(h2, c2, "42")
	clock = clock.Add(5*time.Minute + time.Second)
	tg2.err = nil
	if w := getAvatar(h2, c2, "42"); w.Code != 200 {
		t.Fatalf("capped: %d", w.Code)
	}
}

func TestAvatarSingleflightAndSemaphore(t *testing.T) {
	clock := now
	tg := &fakeAvatars{photos: map[int64][]byte{42: avatarPNG}, delay: 50 * time.Millisecond}
	ids := make([]int64, 12)
	for i := range ids {
		ids[i] = int64(100 + i)
		tg.photos[ids[i]] = avatarPNG
	}
	h, c := avatarServer(t, tg, &clock, append(ids, 42)...)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if w := getAvatar(h, c, "42"); w.Code != 200 {
				t.Errorf("code %d", w.Code)
			}
		})
	}
	wg.Wait()
	if tg.calls.Load() != 1 {
		t.Fatalf("same id: telegram calls = %d", tg.calls.Load())
	}

	tg.calls.Store(0)
	for _, id := range ids {
		wg.Go(func() {
			if w := getAvatar(h, c, strconv.FormatInt(id, 10)); w.Code != 200 {
				t.Errorf("code %d", w.Code)
			}
		})
	}
	wg.Wait()
	if tg.calls.Load() != 12 || tg.peak > 4 || tg.peak < 2 {
		t.Fatalf("calls %d peak %d", tg.calls.Load(), tg.peak)
	}
}

func TestAvatarRejectsNonImage(t *testing.T) {
	clock := now
	tg := &fakeAvatars{photos: map[int64][]byte{42: []byte("<html>x</html>"), 43: bytes.Repeat(avatarPNG[:8], 1<<18)}}
	h, c := avatarServer(t, tg, &clock, 42, 43)
	for _, id := range []string{"42", "43"} {
		if w := getAvatar(h, c, id); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", id, w.Code)
		}
	}
}

func TestAvatarBot(t *testing.T) {
	clock := now
	tg := &fakeAvatars{botID: 777, photos: map[int64][]byte{777: avatarPNG}}
	h, c := avatarServer(t, tg, &clock)
	if w := getAvatar(h, c, "bot"); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), avatarPNG) {
		t.Fatalf("%d", w.Code)
	}
	tg2 := &fakeAvatars{botErr: errors.New("down")}
	h2, c2 := avatarServer(t, tg2, &clock)
	if w := getAvatar(h2, c2, "bot"); w.Code != http.StatusNotFound {
		t.Fatalf("bot fail: %d", w.Code)
	}
	if w := getAvatar(h, "", "bot"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bot no session: %d", w.Code)
	}
}
