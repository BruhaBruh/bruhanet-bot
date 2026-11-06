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
	err    error
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
