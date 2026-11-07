package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/store"
)

func (f *fakeStore) CreateAdminGifts(_ context.Context, months, count int, at time.Time) ([]model.Gift, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.Gift, 0, count)
	for range count {
		g := model.Gift{
			ID: int64(len(f.gifts) + 1), Code: strings.Repeat("c", 21) + string(rune('a'+len(f.gifts))), Months: months,
			Status: model.GiftReady, ByAdmin: true, CreatedAt: at,
		}
		f.gifts = append(f.gifts, g)
		out = append(out, g)
	}
	return out, nil
}

func (f *fakeStore) CancelGift(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.gifts {
		if f.gifts[i].ID == id {
			if f.gifts[i].Status != model.GiftReady {
				return store.ErrGiftNotActive
			}
			f.gifts[i].Status = model.GiftCancelled
			return nil
		}
	}
	return store.ErrNotFound
}

// DeleteGift повторяет правило допустимых статусов; удалённый исчезает из списка (физически или нет — проверено в store).
func (f *fakeStore) DeleteGift(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.gifts {
		if f.gifts[i].ID == id {
			if st := f.gifts[i].Status; st != model.GiftReady && st != model.GiftCancelled {
				return store.ErrNotDeletable
			}
			f.gifts = slices.Delete(f.gifts, i, i+1)
			return nil
		}
	}
	return store.ErrNotFound
}

// ListGifts повторяет фильтр, порядок и скрытие кода pending (реальный SQL проверен в store-тестах).
func (f *fakeStore) ListGifts(_ context.Context, fl model.GiftFilter) ([]model.Gift, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := []model.Gift{}
	for i := len(f.gifts) - 1; i >= 0; i-- {
		if g := f.gifts[i]; fl.Status == "" || g.Status == fl.Status {
			if g.Status == model.GiftPending {
				g.Code = ""
			}
			all = append(all, g)
		}
	}
	lo := min(fl.Offset, len(all))
	return all[lo:min(lo+fl.Limit, len(all))], len(all), nil
}

type giftsJSON struct {
	Items []struct {
		ID      int64  `json:"id"`
		Code    string `json:"code"`
		Months  int    `json:"months"`
		Status  string `json:"status"`
		ByAdmin bool   `json:"byAdmin"`
	} `json:"items"`
	Total int `json:"total"`
}

func TestGiftsCreateListCancel(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))

	for _, body := range []string{`{"months":0,"count":1}`, `{"months":121,"count":1}`, `{"months":3,"count":0}`, `{"months":3,"count":21}`} {
		if w := send(h, http.MethodPost, "/api/gifts", c, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"invalid"`) {
			t.Errorf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := send(h, http.MethodPost, "/api/gifts", c, `{`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", w.Code)
	}
	w := send(h, http.MethodPost, "/api/gifts", c, `{"months":6,"count":3}`)
	var created giftsJSON
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &created) != nil || len(created.Items) != 3 ||
		created.Items[0].Months != 6 || created.Items[0].Status != "ready" || !created.Items[0].ByAdmin || created.Items[0].Code == "" {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}

	// отмена: готовый → 204, повтор → 409, нет такого → 404, мусор → 400
	id := created.Items[0].ID
	path := "/api/gifts/" + itoa(id) + "/cancel"
	if w = send(h, http.MethodPost, path, c, `{}`); w.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	if w = send(h, http.MethodPost, path, c, `{}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not_active") {
		t.Fatalf("cancel twice: %d %s", w.Code, w.Body)
	}
	if w = send(h, http.MethodPost, "/api/gifts/999/cancel", c, `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("cancel unknown: %d", w.Code)
	}
	if w = send(h, http.MethodPost, "/api/gifts/abc/cancel", c, `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("cancel garbage: %d", w.Code)
	}
	st.gifts = append(st.gifts, model.Gift{ID: 10, Code: "hidden", Months: 1, Status: model.GiftPending})

	// вкладки; код pending не отдаётся
	for q, want := range map[string]int{"": 4, "?status=all": 4, "?status=ready": 2, "?status=cancelled": 1, "?status=redeemed": 0, "?status=pending": 1} {
		w = do(h, http.MethodGet, "/api/gifts"+q, c, "")
		var got giftsJSON
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil || got.Total != want || len(got.Items) != want {
			t.Errorf("%q: %d %s", q, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "hidden") {
			t.Errorf("%q: pending code leaked", q)
		}
	}
	for _, q := range []string{"?status=bogus", "?page=0", "?page=x"} {
		if w = do(h, http.MethodGet, "/api/gifts"+q, c, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%q: %d", q, w.Code)
		}
	}
	if w = do(h, http.MethodGet, "/api/gifts?page=2", c, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("page 2: %d %s", w.Code, w.Body)
	}
}

func TestGiftsNeedSession(t *testing.T) {
	h, _ := newServer(t, "http://localhost:5173")
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/api/gifts"}, {http.MethodPost, "/api/gifts"}, {http.MethodPost, "/api/gifts/1/cancel"},
	} {
		ct := ""
		if rt.method == http.MethodPost {
			ct = "application/json"
		}
		if w := do(h, rt.method, rt.path, "", ct); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: %d", rt.method, rt.path, w.Code)
		}
	}
}

// Подтверждение подарка из панели: ссылка покупателю, панель не тронута (ни срок, ни лимит устройств).
func TestResolveGiftPaymentFromPanel(t *testing.T) {
	panel := &fakePanel{}
	h, st := newServerPanel(t, "http://localhost:5173", panel)
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.payments = []model.Payment{{
		ID: 1, TelegramID: 7, Month: 3, Price: 500, Status: model.PaymentPending, Kind: model.KindGift,
		GiftCode: "SECRETCODESECRETCODE22", AdminMessageID: 5,
	}}
	w := send(h, http.MethodPost, "/api/payments/1/confirm", c, `{}`)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "SECRETCODE") {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	if len(panel.updates) != 0 || len(panel.limits) != 0 || len(st.panelWrites) != 0 {
		t.Fatalf("panel touched: %v %v %v", panel.updates, panel.limits, st.panelWrites)
	}
	var result, closed int
	for _, m := range st.outbox {
		switch m.EventType {
		case payflow.EventGiftResult:
			if m.ChatID != 7 || m.TextKey != "gift.ready" || m.GiftCode != "SECRETCODESECRETCODE22" || m.Months != 3 {
				t.Errorf("result = %+v", m)
			}
			result++
		case payflow.EventAdminClosed:
			closed++
		}
	}
	if result != 1 || closed != 1 {
		t.Fatalf("outbox = %+v", st.outbox)
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func TestDeleteGift(t *testing.T) {
	h, st := newServer(t, "http://localhost:5173")
	c := st.addSession(t, adminID, now.Add(time.Hour))
	st.gifts = []model.Gift{
		{ID: 1, Months: 1, Status: model.GiftReady},
		{ID: 2, Months: 1, Status: model.GiftCancelled},
		{ID: 3, Months: 1, Status: model.GiftRedeemed},
		{ID: 4, Months: 1, Status: model.GiftPending},
	}
	del := func(id string) *httptest.ResponseRecorder { return send(h, http.MethodDelete, "/api/gifts/"+id, c, "") }
	for _, id := range []string{"1", "2"} {
		if w := del(id); w.Code != http.StatusNoContent {
			t.Fatalf("delete %s: %d", id, w.Code)
		}
	}
	for _, id := range []string{"3", "4"} {
		if w := del(id); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not_deletable") {
			t.Fatalf("delete %s: %d %s", id, w.Code, w.Body)
		}
	}
	if del("999").Code != http.StatusNotFound || del("abc").Code != http.StatusBadRequest {
		t.Fatal("unknown / garbage")
	}
	if w := send(h, http.MethodDelete, "/api/gifts/3", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session: %d", w.Code)
	}
	if w := do(h, http.MethodDelete, "/api/gifts/3", c, "text/plain"); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("not json: %d", w.Code)
	}
}
