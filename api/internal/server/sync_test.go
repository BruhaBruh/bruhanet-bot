package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
)

func (f *fakeStore) WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error {
	if f.syncBusy {
		return store.ErrSyncRunning
	}
	return fn(ctx)
}

func (f *fakeStore) ApplyPanelSync(_ context.Context, users []model.PanelUser, _, _ time.Time) (int, error) {
	f.syncApplied++
	f.syncUsers = users
	return len(users), nil
}

func (f *fakeStore) LastPanelSync(context.Context) (model.PanelSync, error) {
	if f.syncLast == nil {
		return model.PanelSync{}, store.ErrNotFound
	}
	return *f.syncLast, nil
}

func TestSyncOK(t *testing.T) {
	h, st := newServerPanel(t, "http://localhost:5173", &fakePanel{ids: []int{7, 0, 8}})
	c := st.addSession(t, adminID, now.Add(time.Hour))
	w := do(h, http.MethodPost, "/api/sync", c, "application/json")
	var out model.PanelSync
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if out.Total != 2 || out.Updated != 2 || !out.FinishedAt.Equal(now) || len(st.syncUsers) != 2 {
		t.Fatalf("%+v %+v", out, st.syncUsers)
	}
}

func TestSyncAlreadyRunning(t *testing.T) {
	h, st := newServerPanel(t, "http://localhost:5173", &fakePanel{ids: []int{7}})
	st.syncBusy = true
	w := do(h, http.MethodPost, "/api/sync", st.addSession(t, adminID, now.Add(time.Hour)), "application/json")
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusConflict || out["error"] != "sync_running" || out["message"] == "" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if st.syncApplied != 0 {
		t.Fatal("applied while busy")
	}
}

func TestSyncPanelDownLeavesDataUntouched(t *testing.T) {
	for name, panel := range map[string]*fakePanel{
		"fails midway": {ids: []int{7}, usersErr: errors.New("boom")},
		"empty":        {},
	} {
		h, st := newServerPanel(t, "http://localhost:5173", panel)
		w := do(h, http.MethodPost, "/api/sync", st.addSession(t, adminID, now.Add(time.Hour)), "application/json")
		var out map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusBadGateway || out["message"] == "" {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		if st.syncApplied != 0 {
			t.Fatalf("%s: applied", name)
		}
	}
}

func TestSyncRequiresSessionAndJSON(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	if w := do(h, http.MethodPost, "/api/sync", "", "application/json"); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/sync", st.addSession(t, adminID, now.Add(time.Hour)), "text/plain"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("not json: %d", w.Code)
	}
}

func TestLastSync(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	get := func() *time.Time {
		t.Helper()
		var out struct {
			LastSyncAt *time.Time `json:"lastSyncAt"`
		}
		w := do(h, http.MethodGet, "/api/sync", c, "")
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != http.StatusOK {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		return out.LastSyncAt
	}
	if at := get(); at != nil {
		t.Fatalf("never synced: %v", at)
	}
	st.syncLast = &model.PanelSync{FinishedAt: now}
	if at := get(); at == nil || !at.Equal(now) {
		t.Fatalf("last: %v", at)
	}
}
