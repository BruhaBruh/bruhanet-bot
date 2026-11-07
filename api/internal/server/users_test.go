package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func cardServer(t *testing.T, p *fakePanel) (http.Handler, *fakeStore, string) {
	t.Helper()
	h, st := newServerPanel(t, "http://localhost:5173", p)
	st.users = []model.UserSummary{{TelegramID: 7, Username: "bob", Confirmed: 1, ConfirmedSum: 150}}
	st.payments = []model.Payment{{ID: 1, TelegramID: 7, Month: 1, Price: 150, Status: model.PaymentConfirmed, CreatedAt: now}}
	return h, st, st.addSession(t, adminID, now.Add(time.Hour))
}

type card struct {
	User     model.UserSummary `json:"user"`
	Stats    model.UserStats   `json:"stats"`
	Payments []model.Payment   `json:"payments"`
	Panel    struct {
		ExpireAt *time.Time `json:"expireAt"`
		Status   *string    `json:"status"`
		Error    *string    `json:"error"`
	} `json:"panel"`
	Referral model.UserReferral `json:"referral"`
}

func getCard(t *testing.T, h http.Handler, c string, wantCode int) card {
	t.Helper()
	w := do(h, http.MethodGet, "/api/users/7", c, "")
	if w.Code != wantCode {
		t.Fatalf("code %d: %s", w.Code, w.Body)
	}
	var out card
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUserCardPanelDown(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{err: errors.New("boom")})
	got := getCard(t, h, c, http.StatusOK)
	if got.Panel.Error == nil || *got.Panel.Error != "panel_unavailable" || got.Panel.Status != nil || got.Panel.ExpireAt != nil {
		t.Fatalf("panel = %+v", got.Panel)
	}
	if got.User.TelegramID != 7 || len(got.Payments) != 1 || got.Stats.Confirmed != 1 {
		t.Fatalf("card = %+v", got)
	}
}

func TestUserCardReferral(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{err: errors.New("boom")})
	by := int64(9)
	st.referral = model.UserReferral{Invited: 4, Paid: 1, ReferredBy: &by, ReferredByUsername: "alice"}
	got := getCard(t, h, c, http.StatusOK)
	if got.Referral.Invited != 4 || got.Referral.Paid != 1 || got.Referral.ReferredBy == nil || *got.Referral.ReferredBy != 9 || got.Referral.ReferredByUsername != "alice" {
		t.Fatalf("referral = %+v", got.Referral)
	}
}

func TestUserCardPanel(t *testing.T) {
	exp := now.Add(48 * time.Hour)
	h, _, c := cardServer(t, &fakePanel{user: &api.UserItemInfo{ExpireAt: exp, Status: api.UserItemInfoStatusACTIVE}})
	got := getCard(t, h, c, http.StatusOK)
	if got.Panel.Error != nil || got.Panel.Status == nil || *got.Panel.Status != "ACTIVE" ||
		got.Panel.ExpireAt == nil || !got.Panel.ExpireAt.Equal(exp) {
		t.Fatalf("panel = %+v", got.Panel)
	}

	h, _, c = cardServer(t, &fakePanel{err: remnawave.ErrUserNotFound})
	got = getCard(t, h, c, http.StatusOK)
	if got.Panel.Error != nil || got.Panel.Status == nil || *got.Panel.Status != "not_found" {
		t.Fatalf("not found panel = %+v", got.Panel)
	}
}

func TestUserCardDevices(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{user: &api.UserItemInfo{ExpireAt: now, Status: api.UserItemInfoStatusACTIVE}})
	w := do(h, http.MethodGet, "/api/users/7", c, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"devices":{"base":5,"extra":0,"price":0}`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// Лимит устройств из самой панели отдаётся в карточке; нет значения — поля нет.
func TestUserCardPanelDeviceLimit(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{user: &api.UserItemInfo{ExpireAt: now, Status: api.UserItemInfoStatusACTIVE, HwidDeviceLimit: api.NilInt{Value: 10}}})
	if w := do(h, http.MethodGet, "/api/users/7", c, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"deviceLimit":10`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	h, _, c = cardServer(t, &fakePanel{user: &api.UserItemInfo{ExpireAt: now, Status: api.UserItemInfoStatusACTIVE, HwidDeviceLimit: api.NilInt{Null: true}}})
	if w := do(h, http.MethodGet, "/api/users/7", c, ""); strings.Contains(w.Body.String(), "deviceLimit") {
		t.Fatalf("no limit in the panel: %s", w.Body)
	}
}

func TestUserCardErrors(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{})
	if w := do(h, http.MethodGet, "/api/users/8", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if w := do(h, http.MethodGet, "/api/users/abc", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
}

type usersPage struct {
	Items []model.UserSummary `json:"items"`
	Total int                 `json:"total"`
	Page  int                 `json:"page"`
	Pages int                 `json:"pages"`
}

func getUsers(t *testing.T, h http.Handler, c, query string) usersPage {
	t.Helper()
	w := do(h, http.MethodGet, "/api/users"+query, c, "")
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", query, w.Code, w.Body)
	}
	var out usersPage
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUsersSearch(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	got := getUsers(t, h, c, "?q=bo")
	if len(got.Items) != 1 || got.Total != 1 || got.Page != 1 || got.Pages != 1 || st.lastUF.Q != "bo" || st.lastUF.Tab != model.UserTabAll {
		t.Fatalf("%+v %+v", got, st.lastUF)
	}
}

func TestUsersPagination(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	st.users = make([]model.UserSummary, 51)
	got := getUsers(t, h, c, "?page=2")
	if len(got.Items) != 1 || got.Total != 51 || got.Page != 2 || got.Pages != 2 || st.lastUF.Offset != 50 || st.lastUF.Limit != 50 {
		t.Fatalf("%d %+v %+v", len(got.Items), got, st.lastUF)
	}
	st.users = make([]model.UserSummary, 50)
	if got = getUsers(t, h, c, ""); got.Pages != 1 {
		t.Fatalf("exactly a page: %+v", got.Pages)
	}
	st.users = nil
	if got = getUsers(t, h, c, ""); got.Pages != 1 {
		t.Fatalf("empty: %+v", got)
	}
}

func TestUsersFilters(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	getUsers(t, h, c, "?tab=blocked_bot&hasPayments=1&hasUnpaidAdvances=true&from=2026-10-01&to=2026-10-31")
	f := st.lastUF
	wantFrom := time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC) // полночь МСК
	wantTo := time.Date(2026, 10, 31, 21, 0, 0, 0, time.UTC)  // to включительно
	if f.Tab != model.UserTabBlockedBot || !f.HasPayments || !f.HasUnpaidAdvances || !f.From.Equal(wantFrom) || !f.To.Equal(wantTo) || !f.Now.Equal(now) {
		t.Fatalf("%+v", f)
	}
	getUsers(t, h, c, "?tab=active")
	if f = st.lastUF; f.Tab != model.UserTabActive || f.HasPayments || !f.From.IsZero() || !f.To.IsZero() {
		t.Fatalf("%+v", f)
	}
	for _, q := range []string{"?tab=nope", "?page=0", "?page=x", "?hasPayments=maybe", "?hasUnpaidAdvances=2", "?from=01.10.2026", "?to=x", "?from=2026-10-02&to=2026-10-01"} {
		if w := do(h, http.MethodGet, "/api/users"+q, c, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
}

func TestUsersTrialTab(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	st.users = []model.UserSummary{{TelegramID: 7, Username: "bob", InTrial: true}}
	got := getUsers(t, h, c, "?tab=trial")
	if len(got.Items) != 1 || got.Total != 1 || st.lastUF.Tab != model.UserTabTrial || !got.Items[0].InTrial {
		t.Fatalf("%+v %+v", got, st.lastUF)
	}
}

func TestUserCardPromosAndPanelID(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{user: &api.UserItemInfo{ID: 1234, ExpireAt: now, Status: api.UserItemInfoStatusACTIVE}})
	st.userPromos = []model.UserPromo{{Code: "AAA", Kind: model.PromoPercent, Value: 10, ActivatedAt: now}}
	w := do(h, http.MethodGet, "/api/users/7", c, "")
	var out struct {
		Promos []model.UserPromo `json:"promos"`
		Panel  struct {
			ID *int `json:"id"`
		} `json:"panel"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Promos) != 1 || out.Promos[0].Code != "AAA" || out.Panel.ID == nil || *out.Panel.ID != 1234 {
		t.Fatalf("%s", w.Body)
	}

	h, _, c = cardServer(t, &fakePanel{err: remnawave.ErrUserNotFound})
	w = do(h, http.MethodGet, "/api/users/7", c, "")
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Panel.ID != nil {
		t.Fatalf("not found id: %s", w.Body)
	}
}

func TestUserCardPanelTimeout(t *testing.T) {
	h, st := newServerTimeout(t, "http://localhost:5173", blockingPanel{}, 20*time.Millisecond)
	st.users = []model.UserSummary{{TelegramID: 7, Username: "bob"}}
	got := getCard(t, h, st.addSession(t, adminID, now.Add(time.Hour)), http.StatusOK)
	if got.Panel.Error == nil || *got.Panel.Error != "panel_unavailable" {
		t.Fatalf("panel = %+v", got.Panel)
	}
}
