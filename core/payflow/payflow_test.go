package payflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

var at = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

type fakeStore struct {
	p      model.Payment
	status model.PaymentStatus
	out    []model.OutboxMessage
}

func (f *fakeStore) ResolvePayment(_ context.Context, _ int64, status model.PaymentStatus, _ time.Time,
	apply func(model.Payment) ([]model.OutboxMessage, error),
) (model.Payment, error) {
	msgs, err := apply(f.p)
	if err != nil {
		return model.Payment{}, err
	}
	f.status, f.out = status, msgs
	return f.p, nil
}

type fakePanel struct {
	expire time.Time
	limits []int
	err    error
}

func (f *fakePanel) SetDeviceLimit(_ context.Context, _ int64, limit int) (*api.UserItemInfo, error) {
	f.limits = append(f.limits, limit)
	return &api.UserItemInfo{}, f.err
}

func (f *fakePanel) UpdateUser(_ context.Context, _ int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	f.expire = expireAt
	return &api.UserItemInfo{}, f.err
}

func TestResolveDates(t *testing.T) {
	future := at.Add(5 * 24 * time.Hour)
	past := at.Add(-90 * 24 * time.Hour)
	for _, tc := range []struct {
		name    string
		prev    time.Time
		approve bool
		want    time.Time
		status  model.PaymentStatus
		key     string
	}{
		{"confirm from prev", future, true, future.Add(6 * payflow.Month), model.PaymentConfirmed, "payment.confirmed"},
		{"confirm expired counts from now", past, true, at.Add(6 * payflow.Month), model.PaymentConfirmed, "payment.confirmed"},
		{"decline returns prev", future, false, future, model.PaymentDeclined, "payment.declined"},
		{"decline expired keeps prev", past, false, past, model.PaymentDeclined, "payment.declined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeStore{p: model.Payment{ID: 1, TelegramID: 7, Month: 6, Price: 800, PrevExpireAt: tc.prev, AdminMessageID: 5}}
			panel := &fakePanel{}
			if _, err := payflow.Resolve(t.Context(), st, panel, payflow.Decision{ID: 1, Approve: tc.approve, At: at}); err != nil {
				t.Fatal(err)
			}
			if !panel.expire.Equal(tc.want) || st.status != tc.status {
				t.Fatalf("expire = %v, status = %s", panel.expire, st.status)
			}
			// решение из бота: сообщение закрывает хендлер, в outbox только уведомление юзеру
			if len(st.out) != 1 {
				t.Fatalf("outbox = %+v", st.out)
			}
			m := st.out[0]
			if m.EventType != payflow.EventResult || m.ChatID != 7 || m.TextKey != tc.key || !m.ExpireAt.Equal(tc.want) || m.ParseMode != "HTML" {
				t.Fatalf("message = %+v", m)
			}
		})
	}
}

func TestResolveFromPanelClosesAdminMessage(t *testing.T) {
	st := &fakeStore{p: model.Payment{ID: 1, TelegramID: 7, Month: 6, Price: 800, PrevExpireAt: at, AdminMessageID: 5}}
	if _, err := payflow.Resolve(t.Context(), st, &fakePanel{}, payflow.Decision{ID: 1, Approve: true, At: at, AdminID: 99}); err != nil {
		t.Fatal(err)
	}
	if len(st.out) != 2 {
		t.Fatalf("outbox = %+v", st.out)
	}
	m := st.out[1]
	if m.EventType != payflow.EventAdminClosed || m.ChatID != 99 || m.MessageID != 5 || !m.Confirmed || m.Months != 6 || m.Price != 800 {
		t.Fatalf("message = %+v", m)
	}
}

func TestResolvePanelError(t *testing.T) {
	st := &fakeStore{p: model.Payment{ID: 1, TelegramID: 7, Month: 1, PrevExpireAt: at}}
	boom := errors.New("panel is down")
	_, err := payflow.Resolve(t.Context(), st, &fakePanel{err: boom}, payflow.Decision{ID: 1, Approve: true, At: at, AdminID: 99})
	if !errors.Is(err, payflow.ErrPanel) || !errors.Is(err, boom) || st.status != "" || st.out != nil {
		t.Fatalf("err = %v, status = %q, out = %+v", err, st.status, st.out)
	}
}

func TestResolveDevices(t *testing.T) {
	prev := at.Add(5 * 24 * time.Hour)
	for _, tc := range []struct {
		name    string
		approve bool
		limits  []int
		key     string
	}{
		{"confirm writes limit only", true, []int{7}, "payment.devices_confirmed"},
		{"decline touches nothing", false, nil, "payment.devices_declined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeStore{p: model.Payment{ID: 1, TelegramID: 7, Kind: model.KindDevices, ExtraDevices: 2, Price: 90, PrevExpireAt: prev, UserExtra: 0}}
			panel := &fakePanel{}
			if _, err := payflow.Resolve(t.Context(), st, panel, payflow.Decision{ID: 1, Approve: tc.approve, At: at, DevicesBase: 5}); err != nil {
				t.Fatal(err)
			}
			if !panel.expire.IsZero() {
				t.Fatalf("term written: %v", panel.expire)
			}
			if len(panel.limits) != len(tc.limits) || (len(tc.limits) > 0 && panel.limits[0] != tc.limits[0]) {
				t.Fatalf("limits = %v", panel.limits)
			}
			m := st.out[0]
			if m.TextKey != tc.key || m.Devices != 2 || !m.ExpireAt.IsZero() {
				t.Fatalf("message = %+v", m)
			}
		})
	}
}

func TestResolveDevicesAddsToCurrent(t *testing.T) {
	st := &fakeStore{p: model.Payment{ID: 1, TelegramID: 7, Kind: model.KindDevices, ExtraDevices: 1, UserExtra: 3}}
	panel := &fakePanel{}
	if _, err := payflow.Resolve(t.Context(), st, panel, payflow.Decision{ID: 1, Approve: true, At: at, DevicesBase: 5}); err != nil {
		t.Fatal(err)
	}
	if len(panel.limits) != 1 || panel.limits[0] != 9 {
		t.Fatalf("limits = %v", panel.limits)
	}
}

func TestResolveTariffDevices(t *testing.T) {
	base := model.Payment{ID: 1, TelegramID: 7, Month: 1, PrevExpireAt: at}
	for _, tc := range []struct {
		name         string
		extra, cur   int
		approve      bool
		limits       []int
		panelBaseErr bool
	}{
		{"unchanged", 2, 2, true, nil, false},
		{"increase", 3, 1, true, []int{8}, false},
		{"drop to base", 0, 2, true, []int{5}, false},
		{"decline", 0, 2, false, nil, false},
		{"no base", 1, 0, true, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.ExtraDevices, p.UserExtra = tc.extra, tc.cur
			st := &fakeStore{p: p}
			panel := &fakePanel{}
			d := payflow.Decision{ID: 1, Approve: tc.approve, At: at, DevicesBase: 5}
			if tc.panelBaseErr {
				d.DevicesBase = 0
			}
			_, err := payflow.Resolve(t.Context(), st, panel, d)
			if tc.panelBaseErr {
				if !errors.Is(err, payflow.ErrDevicesBase) || st.status != "" {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || len(panel.limits) != len(tc.limits) || (len(tc.limits) > 0 && panel.limits[0] != tc.limits[0]) {
				t.Fatalf("err %v limits = %v", err, panel.limits)
			}
		})
	}
}
