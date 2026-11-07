package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
	"github.com/bruhabruh/bruhanet/core/store"
)

func (f *fakeStore) UserByUsername(_ context.Context, name string) (model.UserSummary, error) {
	name = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "@"))
	for _, u := range f.users {
		if name != "" && strings.ToLower(u.Username) == name {
			return u, nil
		}
	}
	return model.UserSummary{}, store.ErrNotFound
}

func (f *fakeStore) CreatePromo(_ context.Context, p model.Promo) (model.Promo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p.Code = promo.Normalize(p.Code)
	for _, e := range f.promos {
		if e.Code == p.Code {
			return model.Promo{}, errors.Join(store.ErrPromoExists, errors.New("dup"))
		}
	}
	p.ID, p.CreatedAt = int64(len(f.promos)+1), now
	f.promos = append(f.promos, model.PromoWithUsage{Promo: p})
	return p, nil
}

func (f *fakeStore) PromoByID(_ context.Context, id int64) (model.Promo, error) {
	for _, p := range f.promos {
		if p.ID == id {
			return p.Promo, nil
		}
	}
	return model.Promo{}, store.ErrNotFound
}

// ListPromos повторяет фильтры core/store (реальный SQL проверен в store-тестах).
func (f *fakeStore) ListPromos(_ context.Context, fl model.PromoFilter, at time.Time) ([]model.PromoWithUsage, error) {
	out := []model.PromoWithUsage{}
	for i := len(f.promos) - 1; i >= 0; i-- {
		p := f.promos[i]
		expired := p.ExpiresAt != nil && !p.ExpiresAt.After(at)
		active := !p.Disabled && !expired && (p.MaxActivations == nil || p.Used < *p.MaxActivations)
		switch {
		case fl.Status == "active" && !active,
			fl.Status == "expired" && (p.Disabled || !expired),
			fl.Status == "disabled" && !p.Disabled,
			fl.Kind != "" && string(p.Kind) != fl.Kind,
			fl.Target == "all" && (p.TelegramID != nil || p.OnlyNew),
			fl.Target == "new" && (p.TelegramID != nil || !p.OnlyNew),
			fl.Target == "user" && p.TelegramID == nil:
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeStore) EnablePromo(_ context.Context, id int64) error {
	for i := range f.promos {
		if f.promos[i].ID == id {
			f.promos[i].Disabled = false
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeStore) DisablePromo(_ context.Context, id int64) error {
	for i := range f.promos {
		if f.promos[i].ID == id {
			f.promos[i].Disabled = true
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeStore) PromoActivations(_ context.Context, id int64) ([]model.PromoActivation, error) {
	out := []model.PromoActivation{}
	for _, a := range f.acts {
		if a.PromoID == id {
			out = append(out, a)
		}
	}
	return out, nil
}

type promoJSON struct {
	ID             int64      `json:"id"`
	Code           string     `json:"code"`
	Kind           string     `json:"kind"`
	Value          int        `json:"value"`
	MaxActivations *int       `json:"maxActivations"`
	Used           int        `json:"used"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	ValidFrom      *string    `json:"validFrom"`
	StartsAt       *time.Time `json:"startsAt"`
	ValidUntil     *string    `json:"validUntil"`
	OnlyNew        bool       `json:"onlyNew"`
	TelegramID     *int64     `json:"telegramId"`
	Username       *string    `json:"username"`
	Disabled       bool       `json:"disabled"`
}

func promoServer(t *testing.T) (http.Handler, *fakeStore, string) {
	t.Helper()
	h, st := newServer(t, "http://localhost:5173")
	st.users = []model.UserSummary{{TelegramID: 7, Username: "Bob"}}
	return h, st, st.addSession(t, adminID, now.Add(time.Hour))
}

func createPromo(t *testing.T, h http.Handler, c, body string, want int) promoJSON {
	t.Helper()
	w := send(h, http.MethodPost, "/api/promos", c, body)
	if w.Code != want {
		t.Fatalf("%s: %d %s", body, w.Code, w.Body)
	}
	var out struct {
		Promo promoJSON `json:"promo"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Promo
}

func TestCreatePromo(t *testing.T) {
	h, st, c := promoServer(t)

	// код не задан → сгенерирован; expiresAt — конец дня МСК
	p := createPromo(t, h, c, `{"kind":"percent","value":20,"maxActivations":5,"validUntil":"2026-12-31","onlyNew":true}`, http.StatusCreated)
	if len(p.Code) != 8 || p.Kind != "percent" || p.Value != 20 || !p.OnlyNew || p.MaxActivations == nil || *p.MaxActivations != 5 || p.ID != 1 {
		t.Fatalf("promo = %+v", p)
	}
	if want := time.Date(2027, 1, 1, 0, 0, 0, 0, time.FixedZone("MSK", 3*3600)); p.ExpiresAt == nil || !p.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt = %v, want %v", p.ExpiresAt, want)
	}

	// 2026-12-31 21:00 UTC = 2027-01-01 00:00 МСК: validUntil считается в МСК, а не в UTC
	if p.ValidUntil == nil || *p.ValidUntil != "2026-12-31" {
		t.Fatalf("validUntil = %v", p.ValidUntil)
	}

	// свой код нормализуется; null-поля, user "" = не персональный
	p = createPromo(t, h, c, `{"code":" summer ","kind":"days","value":7,"maxActivations":null,"validUntil":null,"user":""}`, http.StatusCreated)
	if p.Code != "SUMMER" || p.MaxActivations != nil || p.ExpiresAt != nil || p.ValidUntil != nil || p.TelegramID != nil {
		t.Fatalf("promo = %+v", p)
	}

	// @username найден (без учёта регистра) → telegramId + username
	p = createPromo(t, h, c, `{"code":"BOB1","kind":"rub","value":50,"user":"@bob"}`, http.StatusCreated)
	if p.TelegramID == nil || *p.TelegramID != 7 || p.Username == nil || *p.Username != "Bob" {
		t.Fatalf("promo = %+v", p)
	}
	// число — как есть, username подтягивается из users
	p = createPromo(t, h, c, `{"code":"BOB2","kind":"rub","value":50,"user":"7"}`, http.StatusCreated)
	if p.TelegramID == nil || *p.TelegramID != 7 || p.Username == nil || *p.Username != "Bob" {
		t.Fatalf("promo = %+v", p)
	}
	// у юзера нет username → null, не ""
	st.users = append(st.users, model.UserSummary{TelegramID: 8})
	p = createPromo(t, h, c, `{"code":"NONAME","kind":"rub","value":50,"user":"8"}`, http.StatusCreated)
	if p.Username != nil {
		t.Fatalf("username = %v", *p.Username)
	}
	// числа нет в users — не ошибка, username null
	p = createPromo(t, h, c, `{"code":"GHOST","kind":"rub","value":50,"user":"999"}`, http.StatusCreated)
	if p.TelegramID == nil || *p.TelegramID != 999 || p.Username != nil {
		t.Fatalf("promo = %+v", p)
	}
	if len(st.promos) != 6 {
		t.Fatalf("stored %d", len(st.promos))
	}
}

func TestCreatePromoValidFrom(t *testing.T) {
	h, _, c := promoServer(t)
	p := createPromo(t, h, c, `{"kind":"days","value":3,"validFrom":"2026-12-01","validUntil":"2026-12-01"}`, http.StatusCreated)
	if want := time.Date(2026, 12, 1, 0, 0, 0, 0, time.FixedZone("MSK", 3*3600)); p.StartsAt == nil || !p.StartsAt.Equal(want) {
		t.Fatalf("startsAt = %v, want %v", p.StartsAt, want)
	}
	if p.ValidFrom == nil || *p.ValidFrom != "2026-12-01" || p.ValidUntil == nil || *p.ValidUntil != "2026-12-01" {
		t.Fatalf("dates = %v %v", p.ValidFrom, p.ValidUntil)
	}
	if p = createPromo(t, h, c, `{"kind":"days","value":3}`, http.StatusCreated); p.ValidFrom != nil || p.StartsAt != nil {
		t.Fatalf("no start: %+v", p)
	}
	for body, msg := range map[string]string{
		`{"kind":"days","value":3,"validFrom":"2026-12-02","validUntil":"2026-12-01"}`: "позже даты окончания",
		`{"kind":"days","value":3,"validFrom":"01.12.2026"}`:                           "формате",
	} {
		if w := send(h, http.MethodPost, "/api/promos", c, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), msg) {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
}

func TestEnablePromo(t *testing.T) {
	h, st, c := promoServer(t)
	st.promos = []model.PromoWithUsage{{Promo: model.Promo{ID: 1, Code: "OFF", Kind: model.PromoDays, Value: 3, Disabled: true}}}
	for range 2 { // повтор — тоже 204
		if w := send(h, http.MethodPost, "/api/promos/1/enable", c, ""); w.Code != http.StatusNoContent || st.promos[0].Disabled {
			t.Fatalf("enable: %d", w.Code)
		}
	}
	if w := send(h, http.MethodPost, "/api/promos/99/enable", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
	if w := send(h, http.MethodPost, "/api/promos/x/enable", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestCreatePromoRejects(t *testing.T) {
	h, st, c := promoServer(t)
	createPromo(t, h, c, `{"code":"DUP","kind":"days","value":3}`, http.StatusCreated)

	w := send(h, http.MethodPost, "/api/promos", c, `{"code":"dup","kind":"days","value":3}`)
	if w.Code != http.StatusConflict || strings.TrimSpace(w.Body.String()) != `{"error":"exists","message":"Такой код уже есть"}` {
		t.Fatalf("dup: %d %s", w.Code, w.Body)
	}

	bad := map[string]string{
		`{"kind":"percent","value":0}`:                        "",
		`{"kind":"percent","value":101}`:                      "",
		`{"kind":"rub","value":1000001}`:                      "",
		`{"kind":"days","value":3651}`:                        "",
		`{"kind":"gift","value":5}`:                           "",
		`{"code":"a!","kind":"days","value":5}`:               "",
		`{"kind":"days","value":5,"maxActivations":0}`:        "",
		`{"kind":"days","value":5,"validUntil":"2026-11-01"}`: "дата уже прошла",
		`{"kind":"days","value":5,"validUntil":"02.11.2026"}`: "",
		`{"kind":"days","value":5,"user":"@nobody"}`:          "юзер ещё не писал боту, укажи telegram ID",
		`{"kind":"days","value":5,"user":"-5"}`:               "",
		`{"kind":"days","value":5,"user":"bob bob"}`:          "",
		`nope`: "",
	}
	for body, msg := range bad {
		w := send(h, http.MethodPost, "/api/promos", c, body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
		if msg != "" && !strings.Contains(w.Body.String(), msg) {
			t.Fatalf("%s: %s, want %q", body, w.Body, msg)
		}
	}
	if w := send(h, http.MethodPost, "/api/promos", c, `{"kind":"percent","value":101}`); strings.Contains(w.Body.String(), `\n`) || strings.Contains(w.Body.String(), promo.ErrInvalid.Error()) {
		t.Fatalf("message leaks sentinel: %s", w.Body)
	}
	// дешёвая валидация идёт до поиска юзера: плохое значение + неизвестный @user → ошибка значения
	if w := send(h, http.MethodPost, "/api/promos", c, `{"kind":"percent","value":0,"user":"@nobody"}`); strings.Contains(w.Body.String(), "юзер") {
		t.Fatalf("lookup before validation: %s", w.Body)
	}
	if w := send(h, http.MethodPost, "/api/promos", c, `{"kind":"days","value":5,"note":"`+strings.Repeat("x", 5000)+`"}`); !strings.Contains(w.Body.String(), "слишком большой запрос") {
		t.Fatalf("limit: %d %s", w.Code, w.Body)
	}
	if len(st.promos) != 1 {
		t.Fatalf("stored %d", len(st.promos))
	}
	// «до конца сегодняшнего дня МСК» (now = 2026-11-02 15:00 МСК) ещё действует
	createPromo(t, h, c, `{"kind":"days","value":5,"validUntil":"2026-11-02"}`, http.StatusCreated)
}

func TestCreatePromoGenerateRetry(t *testing.T) {
	h, st, c := promoServer(t)
	// коллизии генератора не воспроизвести детерминированно; проверяем, что много подряд сгенерированных уникальны
	seen := map[string]bool{}
	for range 30 {
		p := createPromo(t, h, c, `{"kind":"days","value":1}`, http.StatusCreated)
		if seen[p.Code] {
			t.Fatalf("dup generated %s", p.Code)
		}
		seen[p.Code] = true
	}
	if len(st.promos) != 30 {
		t.Fatalf("stored %d", len(st.promos))
	}
}

func TestListDisableActivations(t *testing.T) {
	h, st, c := promoServer(t)
	past := now.Add(-time.Hour)
	max1 := 1
	st.promos = []model.PromoWithUsage{
		{Promo: model.Promo{ID: 1, Code: "LIVE", Kind: model.PromoDays, Value: 3, CreatedAt: now}},
		{Promo: model.Promo{ID: 2, Code: "OFF", Kind: model.PromoDays, Value: 3, Disabled: true, CreatedAt: now}},
		{Promo: model.Promo{ID: 3, Code: "OLD", Kind: model.PromoDays, Value: 3, ExpiresAt: &past, CreatedAt: now}},
		{Promo: model.Promo{ID: 4, Code: "FULL", Kind: model.PromoDays, Value: 3, MaxActivations: &max1, CreatedAt: now}, Used: 1},
		{Promo: model.Promo{ID: 5, Code: "MINE", Kind: model.PromoRub, Value: 3, TelegramID: ptrI64(7), CreatedAt: now}},
	}
	codes := func(target string) []string {
		var out struct {
			Items []promoJSON `json:"items"`
		}
		getJSON(t, h, target, c, &out)
		cs := make([]string, 0, len(out.Items))
		for _, p := range out.Items {
			cs = append(cs, p.Code)
		}
		return cs
	}
	if got := strings.Join(codes("/api/promos?status=active"), ","); got != "MINE,LIVE" {
		t.Fatalf("active = %s", got)
	}
	for _, q := range []string{"", "?status=all"} {
		if got := strings.Join(codes("/api/promos"+q), ","); got != "MINE,FULL,OLD,OFF,LIVE" {
			t.Fatalf("all%s = %s", q, got)
		}
	}
	var out struct {
		Items []promoJSON `json:"items"`
	}
	getJSON(t, h, "/api/promos", c, &out)
	if out.Items[0].Username == nil || *out.Items[0].Username != "Bob" || out.Items[1].Used != 1 {
		t.Fatalf("items = %+v", out.Items)
	}
	for _, q := range []string{"status=x", "kind=gift", "target=x"} {
		if w := send(h, http.MethodGet, "/api/promos?"+q, c, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", q, w.Code)
		}
	}
	for q, want := range map[string]string{
		"status=expired":                  "OLD",
		"status=disabled":                 "OFF",
		"kind=rub":                        "MINE",
		"target=user":                     "MINE",
		"target=all&status=active":        "LIVE",
		"status=all&kind=days&target=new": "",
	} {
		if got := strings.Join(codes("/api/promos?"+q), ","); got != want {
			t.Fatalf("%s = %q, want %q", q, got, want)
		}
	}

	// disable
	if w := send(h, http.MethodPost, "/api/promos/99/disable", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
	if w := send(h, http.MethodPost, "/api/promos/x/disable", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := send(h, http.MethodPost, "/api/promos/1/disable", c, ""); w.Code != http.StatusNoContent || !st.promos[0].Disabled {
		t.Fatalf("disable: %d", w.Code)
	}

	// activations
	st.acts = []model.PromoActivation{{ID: 1, PromoID: 1, TelegramID: 7, Username: "Bob", PaymentID: ptrI64(3), CreatedAt: now}}
	w := send(h, http.MethodGet, "/api/promos/1/activations", c, "")
	want := `{"items":[{"id":1,"promoId":1,"telegramId":7,"username":"Bob","name":"","paymentId":3,"createdAt":"2026-11-02T12:00:00Z"}]}`
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != want {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := send(h, http.MethodGet, "/api/promos/2/activations", c, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `{"items":[]}`) {
		t.Fatalf("empty: %d %s", w.Code, w.Body)
	}
	if w := send(h, http.MethodGet, "/api/promos/99/activations", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestPromosRequireSession(t *testing.T) {
	h, _, _ := promoServer(t)
	if w := do(h, http.MethodGet, "/api/promos", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d", w.Code)
	}
}

func ptrI64(v int64) *int64 { return &v }
