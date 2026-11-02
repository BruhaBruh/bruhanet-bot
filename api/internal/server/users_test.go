package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
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

func TestUserCardErrors(t *testing.T) {
	h, _, c := cardServer(t, &fakePanel{})
	if w := do(h, http.MethodGet, "/api/users/8", c, ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if w := do(h, http.MethodGet, "/api/users/abc", c, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
}

func TestUsersSearch(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	w := do(h, http.MethodGet, "/api/users?q=bo", c, "")
	var out struct{ Items []model.UserSummary }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 || len(out.Items) != 1 || st.lastQ != "bo" {
		t.Fatalf("%d %s q=%q", w.Code, w.Body, st.lastQ)
	}
}
