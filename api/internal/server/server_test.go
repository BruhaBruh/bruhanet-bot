package server_test

import (
	"context"
	"encoding/hex"
	"io"
	"iter"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/api/internal/server"
	"github.com/bruhabruh/bruhanet/core/auth"
	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/store"
)

const adminID = 1

var now = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

var testSquad = uuid.MustParse("7d1f5b9e-0000-4000-8000-000000000001")

const indexHTML = "<!doctype html><title>test</title>"

// fakeStore повторяет семантику core/store в памяти.
type fakeStore struct {
	mu       sync.Mutex
	tokens   map[string]time.Time // hash → expires_at; удалённый = использованный
	sessions map[string]session

	users    []model.UserSummary
	payments []model.Payment // по возрастанию id
	revenue  int
	lastUF   model.UserFilter
	lastF    model.PaymentFilter
	lastRev  [2]time.Time

	overrides map[string]string
	setRaw    map[string][]byte
	outbox    []model.OutboxMessage

	broadcasts   []fakeBroadcast // по порядку создания
	bcRecipients []int64         // кого «нашла» отправка
	counts       model.AudienceCounts
	lastDays     int
	lastBF       model.BroadcastFilter
	styles       map[string]string
	directs      []model.DirectMessage

	userPromos []model.UserPromo

	syncBusy    bool
	syncUsers   []model.PanelUser
	syncApplied int
	syncLast    *model.PanelSync

	promos []model.PromoWithUsage // по возрастанию id
	acts   []model.PromoActivation

	panelWrites []panelWrite
}

type panelWrite struct {
	id  int64
	exp time.Time
}

type fakeBroadcast struct {
	b          model.Broadcast
	key        uuid.UUID
	recipients []int64
	adminID    int64
}

type session struct {
	id  int64
	exp time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{tokens: map[string]time.Time{}, sessions: map[string]session{}}
}

func (f *fakeStore) addToken(t *testing.T, exp time.Time) string {
	t.Helper()
	raw, hash, err := auth.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens[hex.EncodeToString(hash)] = exp
	return raw
}

func (f *fakeStore) addSession(t *testing.T, id int64, exp time.Time) string {
	t.Helper()
	raw, hash, err := auth.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[hex.EncodeToString(hash)] = session{id: id, exp: exp}
	return raw
}

func (f *fakeStore) sessionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sessions)
}

func (f *fakeStore) ConsumeLoginToken(_ context.Context, hash []byte, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := hex.EncodeToString(hash)
	exp, ok := f.tokens[k]
	if !ok || !exp.After(now) {
		return store.ErrTokenInvalid
	}
	delete(f.tokens, k)
	return nil
}

func (f *fakeStore) CreateSession(_ context.Context, hash []byte, id int64, exp time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[hex.EncodeToString(hash)] = session{id: id, exp: exp}
	return nil
}

func (f *fakeStore) SessionTelegramID(_ context.Context, hash []byte, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[hex.EncodeToString(hash)]
	if !ok || !s.exp.After(now) {
		return 0, store.ErrNotFound
	}
	return s.id, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, hex.EncodeToString(hash))
	return nil
}

func (f *fakeStore) TextOverrides(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	maps.Copy(out, f.overrides)
	return out, nil
}

func (f *fakeStore) SetText(_ context.Context, key, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.overrides == nil {
		f.overrides = map[string]string{}
	}
	f.overrides[key] = body
	return nil
}

func (f *fakeStore) ResetText(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.overrides, key)
	return nil
}

func (f *fakeStore) ClearTextBody(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.overrides, key)
	return nil
}

func (f *fakeStore) TextStyles(context.Context) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	maps.Copy(out, f.styles)
	return out, nil
}

func (f *fakeStore) SetTextStyle(_ context.Context, key string, style *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.styles == nil {
		f.styles = map[string]string{}
	}
	if style == nil {
		delete(f.styles, key)
	} else {
		f.styles[key] = *style
	}
	return nil
}

func (f *fakeStore) SettingsRaw(context.Context) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]byte{}
	maps.Copy(out, f.setRaw)
	return out, nil
}

func (f *fakeStore) SetSettings(_ context.Context, v map[string][]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setRaw == nil {
		f.setRaw = map[string][]byte{}
	}
	maps.Copy(f.setRaw, v)
	return nil
}

func (f *fakeStore) Enqueue(_ context.Context, m model.OutboxMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outbox = append(f.outbox, m)
	return nil
}

func newServer(t *testing.T, baseURL string) (http.Handler, *fakeStore) {
	t.Helper()
	return newServerPanel(t, baseURL, &fakePanel{})
}

func newServerPanel(t *testing.T, baseURL string, panel server.Panel) (http.Handler, *fakeStore) {
	t.Helper()
	return newServerTimeout(t, baseURL, panel, 0)
}

func newServerTimeout(t *testing.T, baseURL string, panel server.Panel, panelTimeout time.Duration) (http.Handler, *fakeStore) {
	t.Helper()
	h, st, _ := newServerTG(t, baseURL, panel, panelTimeout)
	return h, st
}

func newServerTG(t *testing.T, baseURL string, panel server.Panel, panelTimeout time.Duration) (http.Handler, *fakeStore, *fakeTG) {
	t.Helper()
	st := newFakeStore()
	tg := &fakeTG{fileID: "AgAD-file"}
	h := server.New(server.Deps{
		Store:        st,
		Panel:        panel,
		Telegram:     tg,
		Defaults:     settings.Defaults(map[int]int{1: 150, 6: 800}, "+7 900", "Т-Банк", "https://t.me/support"),
		PanelTimeout: panelTimeout,
		Squad:        testSquad,
		AdminID:      adminID,
		BaseURL:      baseURL,
		Now:          func() time.Time { return now },
		Static: fstest.MapFS{
			"index.html":    {Data: []byte(indexHTML)},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
	})
	return h, st, tg
}

func do(h http.Handler, method, target, cookie, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: "bn_session", Value: cookie})
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAPIRequiresSession(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")

	if w := do(h, http.MethodGet, "/api/me", "", ""); w.Code != http.StatusUnauthorized ||
		strings.TrimSpace(w.Body.String()) != `{"error":"unauthorized"}` {
		t.Fatalf("no cookie: %d %s", w.Code, w.Body)
	}
	if w := do(h, http.MethodGet, "/api/me", "garbage", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage cookie: %d", w.Code)
	}
	expired := st.addSession(t, adminID, now.Add(-time.Second))
	if w := do(h, http.MethodGet, "/api/me", expired, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired: %d", w.Code)
	}
	foreign := st.addSession(t, 42, now.Add(time.Hour))
	if w := do(h, http.MethodGet, "/api/me", foreign, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("foreign id: %d", w.Code)
	}

	valid := st.addSession(t, adminID, now.Add(time.Hour))
	w := do(h, http.MethodGet, "/api/me", valid, "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"telegramId":1}` ||
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("valid: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

// TestAPIDefaultDeny — /api/* закрыт сессией целиком, включая ещё не написанные и неизвестные маршруты.
func TestAPIDefaultDeny(t *testing.T) {
	h, _ := newServer(t, "http://localhost:5173")
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodPost, "/api/logout"},
		{http.MethodGet, "/api/"},
		{http.MethodGet, "/api/nope"},
		{http.MethodGet, "/api/logout"},
		{http.MethodDelete, "/api/users/1"},
	} {
		ct := ""
		if rt.method != http.MethodGet {
			ct = "application/json"
		}
		if w := do(h, rt.method, rt.path, "", ct); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without cookie: %d", rt.method, rt.path, w.Code)
		}
	}
}

func TestJSONOnly(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	if w := do(h, http.MethodPost, "/api/logout", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/logout", c, ""); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("no content-type: %d", w.Code)
	}
	if st.sessionCount() != 1 {
		t.Fatal("rejected request must not log out")
	}
	if w := do(h, http.MethodPost, "/api/logout", c, "application/json; charset=utf-8"); w.Code != http.StatusNoContent {
		t.Fatalf("json: %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	w := do(h, http.MethodPost, "/api/logout", c, "application/json")
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	sc := w.Header().Get("Set-Cookie")
	if !strings.Contains(sc, "bn_session=") || !strings.Contains(sc, "Max-Age=0") {
		t.Fatalf("set-cookie = %q", sc)
	}
	if w := do(h, http.MethodGet, "/api/me", c, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", w.Code)
	}
}

func (f *fakeStore) ListUsers(_ context.Context, fl model.UserFilter) ([]model.UserSummary, int, error) {
	f.lastUF = fl
	from := min(fl.Offset, len(f.users))
	return f.users[from:min(from+fl.Limit, len(f.users))], len(f.users), nil
}

func (f *fakeStore) UserPromos(context.Context, int64) ([]model.UserPromo, error) {
	return f.userPromos, nil
}

func (f *fakeStore) UserByID(_ context.Context, id int64) (model.UserSummary, error) {
	for _, u := range f.users {
		if u.TelegramID == id {
			return u, nil
		}
	}
	return model.UserSummary{}, store.ErrNotFound
}

func (f *fakeStore) PaymentsByUser(_ context.Context, id int64) ([]model.Payment, error) {
	out := []model.Payment{}
	for i := len(f.payments) - 1; i >= 0; i-- {
		if f.payments[i].TelegramID == id {
			out = append(out, f.payments[i])
		}
	}
	return out, nil
}

func (f *fakeStore) PaymentByID(_ context.Context, id int64) (model.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.payments {
		if p.ID == id {
			return p, nil
		}
	}
	return model.Payment{}, store.ErrNotFound
}

// ResolvePayment — как core/store: ошибка apply ничего не меняет, повтор — ErrNotPending.
func (f *fakeStore) ResolvePayment(_ context.Context, id int64, status model.PaymentStatus, at time.Time,
	apply func(model.Payment) ([]model.OutboxMessage, error),
) (model.Payment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.payments {
		p := &f.payments[i]
		if p.ID != id {
			continue
		}
		if p.Status != model.PaymentPending {
			return *p, store.ErrNotPending
		}
		msgs, err := apply(*p)
		if err != nil {
			return model.Payment{}, err
		}
		p.Status, p.ResolvedAt = status, &at
		f.outbox = append(f.outbox, msgs...)
		return *p, nil
	}
	return model.Payment{}, store.ErrNotFound
}

func (f *fakeStore) SetUserPanel(_ context.Context, id int64, expireAt time.Time, _ string, _ time.Time) error {
	f.panelWrites = append(f.panelWrites, panelWrite{id, expireAt})
	return nil
}

func (f *fakeStore) UserStats(_ context.Context, id, _ int64) (model.UserStats, error) {
	return model.UserStats{Confirmed: 1, ConfirmedSum: 150}, nil
}

func (f *fakeStore) ListPayments(_ context.Context, fl model.PaymentFilter) ([]model.Payment, int, error) {
	f.lastF = fl
	return f.payments, len(f.payments), nil
}

func (f *fakeStore) Revenue(_ context.Context, from, to time.Time) (int, error) {
	f.lastRev = [2]time.Time{from, to}
	return f.revenue, nil
}

type fakePanel struct {
	user     *api.UserItemInfo
	err      error
	ids      []int // telegram ID юзеров панели; 0 — без ID
	usersErr error

	updates   []time.Time
	updateErr error
	squad     uuid.UUID
}

func (p *fakePanel) GetUser(context.Context, int64) (*api.UserItemInfo, error) { return p.user, p.err }

func (p *fakePanel) UpdateUser(_ context.Context, _ int64, _ string, expireAt time.Time, squad uuid.UUID) (*api.UserItemInfo, error) {
	if p.updateErr != nil {
		return nil, p.updateErr
	}
	p.updates = append(p.updates, expireAt)
	p.squad = squad
	return &api.UserItemInfo{ExpireAt: expireAt, Status: api.UserItemInfoStatusACTIVE}, nil
}

func (p *fakePanel) Users(context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		for _, id := range p.ids {
			var u api.UserItemInfo
			if id != 0 {
				u.TelegramId.SetTo(id)
			}
			if !yield(u, nil) {
				return
			}
		}
		if p.usersErr != nil {
			yield(api.UserItemInfo{}, p.usersErr)
		}
	}
}

// blockingPanel висит, пока не отменят контекст.
type blockingPanel struct{}

func (blockingPanel) GetUser(ctx context.Context, _ int64) (*api.UserItemInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingPanel) UpdateUser(ctx context.Context, _ int64, _ string, _ time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingPanel) Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		<-ctx.Done()
		yield(api.UserItemInfo{}, ctx.Err())
	}
}

// SendBroadcast повторяет семантику core/store: ключ, черновик, расписание; получатели — f.bcRecipients.
func (f *fakeStore) SendBroadcast(_ context.Context, d model.BroadcastDraft, admin int64, at time.Time) (model.Broadcast, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.broadcasts {
		if b.key == d.IdempotencyKey {
			return b.b, false, nil
		}
	}
	var idx int
	if d.DraftID != uuid.Nil {
		idx = f.bcIndex(d.DraftID)
		if idx < 0 {
			return model.Broadcast{}, false, store.ErrNotFound
		}
		if f.broadcasts[idx].b.Status != broadcast.StatusDraft {
			return model.Broadcast{}, false, store.ErrBroadcastLocked
		}
	} else {
		f.broadcasts = append(f.broadcasts, fakeBroadcast{b: model.Broadcast{ID: uuid.New(), CreatedAt: now}})
		idx = len(f.broadcasts) - 1
	}
	fb := &f.broadcasts[idx]
	fb.key, fb.adminID = d.IdempotencyKey, admin
	fb.b = fillBroadcast(fb.b, d)
	fb.b.Status = broadcast.StatusScheduled
	if d.ScheduledAt == nil {
		fb.b.Status, fb.b.Total = broadcast.StatusSending, len(f.bcRecipients)
		fb.recipients = f.bcRecipients
	}
	return fb.b, true, nil
}

func fillBroadcast(b model.Broadcast, d model.BroadcastDraft) model.Broadcast {
	b.Text, b.Photo, b.Audience, b.AudienceDays, b.Silent, b.ScheduledAt = d.Text, d.Photo, d.Audience, d.AudienceDays, d.Silent, d.ScheduledAt
	b.Buttons = d.Buttons
	if b.Buttons == nil {
		b.Buttons = []broadcast.Button{}
	}
	b.UpdatedAt = now
	return b
}

func (f *fakeStore) bcIndex(id uuid.UUID) int {
	return slices.IndexFunc(f.broadcasts, func(b fakeBroadcast) bool { return b.b.ID == id })
}

func (f *fakeStore) SaveDraft(_ context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.bcIndex(id)
	if i < 0 {
		f.broadcasts = append(f.broadcasts, fakeBroadcast{b: model.Broadcast{ID: id, Status: broadcast.StatusDraft, CreatedAt: now}})
		i = len(f.broadcasts) - 1
	}
	if f.broadcasts[i].b.Status != broadcast.StatusDraft {
		return model.Broadcast{}, store.ErrBroadcastLocked
	}
	f.broadcasts[i].b = fillBroadcast(f.broadcasts[i].b, d)
	return f.broadcasts[i].b, nil
}

func (f *fakeStore) UpdateScheduled(_ context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.bcIndex(id)
	switch {
	case i < 0:
		return model.Broadcast{}, store.ErrNotFound
	case f.broadcasts[i].b.Status != broadcast.StatusScheduled:
		return model.Broadcast{}, store.ErrBroadcastLocked
	}
	f.broadcasts[i].b = fillBroadcast(f.broadcasts[i].b, d)
	return f.broadcasts[i].b, nil
}

func (f *fakeStore) CancelBroadcast(_ context.Context, id uuid.UUID) (model.Broadcast, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.bcIndex(id)
	if i < 0 {
		return model.Broadcast{}, store.ErrNotFound
	}
	switch st := f.broadcasts[i].b.Status; st {
	case broadcast.StatusDraft, broadcast.StatusScheduled:
		f.broadcasts[i].b.Status = broadcast.StatusCancelled
	case broadcast.StatusCancelled:
	default:
		return model.Broadcast{}, store.ErrBroadcastLocked
	}
	return f.broadcasts[i].b, nil
}

func (f *fakeStore) Broadcast(_ context.Context, id uuid.UUID) (model.Broadcast, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := f.bcIndex(id); i >= 0 {
		return f.broadcasts[i].b, nil
	}
	return model.Broadcast{}, store.ErrNotFound
}

func (f *fakeStore) ListBroadcasts(_ context.Context, bf model.BroadcastFilter) ([]model.Broadcast, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastBF = bf
	var all []model.Broadcast
	for i := len(f.broadcasts) - 1; i >= 0; i-- {
		b := f.broadcasts[i].b
		if len(bf.Statuses) > 0 && !slices.Contains(bf.Statuses, b.Status) {
			continue
		}
		all = append(all, b)
	}
	out := []model.Broadcast{}
	for i := bf.Offset; i < len(all) && len(out) < bf.Limit; i++ {
		out = append(out, all[i])
	}
	return out, len(all), nil
}

func (f *fakeStore) AudienceCounts(_ context.Context, _ time.Time, days int) (model.AudienceCounts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastDays = days
	return f.counts, nil
}

func (f *fakeStore) CreateDirect(_ context.Context, telegramID int64, text, photo string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := int64(len(f.directs) + 1)
	f.directs = append(f.directs, model.DirectMessage{ID: id, TelegramID: telegramID, Text: text, Photo: photo, Status: model.DirectPending, CreatedAt: now})
	return id, nil
}

func (f *fakeStore) DirectMessages(_ context.Context, telegramID int64) ([]model.DirectMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.DirectMessage{}
	for i := len(f.directs) - 1; i >= 0; i-- {
		if f.directs[i].TelegramID == telegramID {
			out = append(out, f.directs[i])
		}
	}
	return out, nil
}

// fakeTG — Telegram для загрузки фото.
type fakeTG struct {
	mu      sync.Mutex
	calls   int
	chatID  int64
	name    string
	caption string
	data    []byte
	fileID  string
	err     error

	file    string // содержимое OpenFile
	opened  string
	openErr error
}

func (f *fakeTG) OpenFile(_ context.Context, fileID string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = fileID
	if f.openErr != nil {
		return nil, f.openErr
	}
	return io.NopCloser(strings.NewReader(f.file)), nil
}

func (f *fakeTG) SendPhoto(_ context.Context, chatID int64, r io.Reader, filename, caption string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.chatID, f.name, f.caption = chatID, filename, caption
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	f.data = data
	return f.fileID, f.err
}
