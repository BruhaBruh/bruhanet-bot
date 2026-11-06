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
)

func TestGrantDays(t *testing.T) {
	exp := now.Add(48 * time.Hour)
	panel := &fakePanel{user: &api.UserItemInfo{ExpireAt: exp, Status: api.UserItemInfoStatusACTIVE}}
	h, st, c := cardServer(t, panel)

	w := send(h, http.MethodPost, "/api/users/7/grant", c, `{"days":7}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	want := exp.Add(7 * 24 * time.Hour)
	if len(panel.updates) != 1 || !panel.updates[0].Equal(want) || panel.squad != testSquad {
		t.Fatalf("panel = %v squad %v", panel.updates, panel.squad)
	}
	if len(st.panelWrites) != 1 || !st.panelWrites[0].exp.Equal(want) {
		t.Fatalf("users не обновлён: %+v", st.panelWrites)
	}
	if len(st.outbox) != 1 || st.outbox[0].EventType != "grant_days" || st.outbox[0].TextKey != "grant.days" ||
		st.outbox[0].ChatID != 7 || st.outbox[0].Days != 7 || !st.outbox[0].ExpireAt.Equal(want) {
		t.Fatalf("outbox = %+v", st.outbox)
	}
	var out struct {
		ExpireAt time.Time `json:"expireAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || !out.ExpireAt.Equal(want) {
		t.Fatalf("body = %s", w.Body)
	}

	// карточка отдаёт историю выдач
	var got struct {
		Grants []model.Grant `json:"grants"`
	}
	cw := do(h, http.MethodGet, "/api/users/7", c, "")
	if err := json.Unmarshal(cw.Body.Bytes(), &got); err != nil || len(got.Grants) != 1 || got.Grants[0].Days != 7 {
		t.Fatalf("card grants = %s", cw.Body)
	}
}

// Срок в прошлом: дни считаются от now.
func TestGrantDaysFromNow(t *testing.T) {
	panel := &fakePanel{user: &api.UserItemInfo{ExpireAt: now.Add(-10 * 24 * time.Hour)}}
	h, _, c := cardServer(t, panel)
	if w := send(h, http.MethodPost, "/api/users/7/grant", c, `{"days":3}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if want := now.Add(3 * 24 * time.Hour); !panel.updates[0].Equal(want) {
		t.Fatalf("panel = %v, want %v", panel.updates, want)
	}
}

func TestGrantDaysValidation(t *testing.T) {
	panel := &fakePanel{}
	h, st, c := cardServer(t, panel)
	for _, body := range []string{`{"days":0}`, `{"days":-1}`, `{"days":366}`, `{}`} {
		if w := send(h, http.MethodPost, "/api/users/7/grant", c, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	for _, body := range []string{`{"days":"7"}`, `nope`, ``} {
		if w := send(h, http.MethodPost, "/api/users/7/grant", c, body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	if w := send(h, http.MethodPost, "/api/users/x/grant", c, `{"days":1}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", w.Code)
	}
	if w := send(h, http.MethodPost, "/api/users/404/grant", c, `{"days":1}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", w.Code)
	}
	if len(panel.updates) != 0 || len(st.grants) != 0 || len(st.outbox) != 0 {
		t.Fatalf("state changed: %v %v %v", panel.updates, st.grants, st.outbox)
	}
	if w := send(h, http.MethodPost, "/api/users/7/grant", "", `{"days":1}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/users/7/grant", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("not json: %d", w.Code)
	}
}

func TestGrantDaysPanelError(t *testing.T) {
	for name, panel := range map[string]*fakePanel{
		"update": {updateErr: errors.New("secret detail")},
		"create": {createErr: errors.New("secret detail")},
	} {
		t.Run(name, func(t *testing.T) {
			h, st, c := cardServer(t, panel)
			w := send(h, http.MethodPost, "/api/users/7/grant", c, `{"days":7}`)
			if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if len(st.grants) != 0 || len(st.outbox) != 0 {
				t.Fatalf("rolled back expected: %v %v", st.grants, st.outbox)
			}
		})
	}
}

func TestGrantDaysPanelTimeout(t *testing.T) {
	h, st := newServerTimeout(t, "http://localhost:5173", blockingPanel{}, 20*time.Millisecond)
	st.users = []model.UserSummary{{TelegramID: 7}}
	c := st.addSession(t, adminID, now.Add(time.Hour))
	if w := send(h, http.MethodPost, "/api/users/7/grant", c, `{"days":1}`); w.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

// Админская выдача работает и для заблокированного вручную юзера.
func TestGrantDaysBlockedUser(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	st.users[0].BlockedAt = &now
	if w := send(h, http.MethodPost, "/api/users/7/grant", c, `{"days":2}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestBlockUnblock(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{user: &api.UserItemInfo{ExpireAt: now}})
	for range 2 { // идемпотентно
		if w := send(h, http.MethodPost, "/api/users/7/block", c, "{}"); w.Code != http.StatusNoContent {
			t.Fatalf("block: %d %s", w.Code, w.Body)
		}
	}
	if st.users[0].BlockedAt == nil || !st.users[0].BlockedAt.Equal(now) {
		t.Fatalf("blocked_at = %v", st.users[0].BlockedAt)
	}
	var got struct {
		User model.UserSummary `json:"user"`
	}
	cw := do(h, http.MethodGet, "/api/users/7", c, "")
	if err := json.Unmarshal(cw.Body.Bytes(), &got); err != nil || got.User.BlockedAt == nil {
		t.Fatalf("card = %s", cw.Body)
	}
	for range 2 {
		if w := send(h, http.MethodPost, "/api/users/7/unblock", c, "{}"); w.Code != http.StatusNoContent {
			t.Fatalf("unblock: %d %s", w.Code, w.Body)
		}
	}
	if st.users[0].BlockedAt != nil {
		t.Fatalf("blocked_at = %v", st.users[0].BlockedAt)
	}
	for _, p := range []string{"block", "unblock"} {
		if w := send(h, http.MethodPost, "/api/users/404/"+p, c, "{}"); w.Code != http.StatusNotFound {
			t.Fatalf("%s unknown: %d", p, w.Code)
		}
		if w := send(h, http.MethodPost, "/api/users/x/"+p, c, "{}"); w.Code != http.StatusBadRequest {
			t.Fatalf("%s bad id: %d", p, w.Code)
		}
		if w := send(h, http.MethodPost, "/api/users/7/"+p, "", "{}"); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s no session: %d", p, w.Code)
		}
	}
}

func TestUsersBlockedTab(t *testing.T) {
	h, st, c := cardServer(t, &fakePanel{})
	if w := do(h, http.MethodGet, "/api/users?tab=blocked", c, ""); w.Code != http.StatusOK || st.lastUF.Tab != model.UserTabBlocked {
		t.Fatalf("%d tab=%q", w.Code, st.lastUF.Tab)
	}
}
