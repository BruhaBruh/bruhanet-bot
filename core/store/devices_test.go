package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
)

func devicesPay(id int64, n int) model.Payment {
	return model.Payment{TelegramID: id, Username: "u", Price: 90, Kind: model.KindDevices, ExtraDevices: n}
}

func withPrev(p model.Payment, prev time.Time) model.Payment {
	p.PrevExpireAt = prev
	return p
}

func noApply(model.Payment) ([]model.OutboxMessage, error) { return nil, nil }

func TestDevicesPaymentRefusedWhilePending(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	tariff, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tariff.Kind != model.KindTariff || tariff.ExtraDevices != 0 {
		t.Fatalf("defaults: %+v", tariff)
	}
	if _, _, err = s.CreatePayment(ctx, devicesPay(1, 2), noRollback, nil); !errors.Is(err, ErrPendingExists) {
		t.Fatalf("err = %v", err)
	}
	// старый pending не тронут
	if got, err := s.PendingPayment(ctx, 1); err != nil || got.ID != tariff.ID {
		t.Fatalf("pending %+v %v", got, err)
	}
}

func TestTariffReplacesPendingDevicesAndReadsPrevAgain(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	stale := time.Now().Add(-100 * time.Hour).Truncate(time.Microsecond)
	fresh := time.Now().Add(72 * time.Hour).Truncate(time.Microsecond)
	dev, _, err := s.CreatePayment(ctx, withPrev(devicesPay(1, 1), stale), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Kind != model.KindDevices || dev.ExtraDevices != 1 || dev.Month != 0 {
		t.Fatalf("devices payment: %+v", dev)
	}
	next := newPay(1, time.Time{})
	next.ExtraDevices = 2
	got, cancelled, err := s.CreatePayment(ctx, next, noRollback, func(context.Context) (time.Time, error) { return fresh, nil })
	if err != nil {
		t.Fatal(err)
	}
	if cancelled == nil || cancelled.ID != dev.ID || !got.PrevExpireAt.Equal(fresh) || got.ExtraDevices != 2 {
		t.Fatalf("got %+v cancelled %+v", got, cancelled)
	}
}

func TestResolveAppliesExtraDevices(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now()
	extra := func() int {
		n, err := s.ExtraDevices(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	resolve := func(p model.Payment, status model.PaymentStatus) {
		t.Helper()
		created, _, err := s.CreatePayment(ctx, p, noRollback, nil)
		if err != nil {
			t.Fatal(err)
		}
		var seen int
		if _, err = s.ResolvePayment(ctx, created.ID, status, now, func(x model.Payment) ([]model.OutboxMessage, error) {
			seen = x.UserExtra
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
		_ = seen
	}

	resolve(devicesPay(1, 2), model.PaymentConfirmed)
	if extra() != 2 {
		t.Fatalf("after +2: %d", extra())
	}
	resolve(devicesPay(1, 1), model.PaymentDeclined)
	if extra() != 2 {
		t.Fatalf("declined must not change: %d", extra())
	}
	resolve(devicesPay(1, 3), model.PaymentConfirmed)
	if extra() != 5 {
		t.Fatalf("after +3: %d", extra())
	}
	tariff := newPay(1, now)
	tariff.ExtraDevices = 1 // продление с сокращением до 1
	resolve(tariff, model.PaymentConfirmed)
	if extra() != 1 {
		t.Fatalf("tariff sets absolute: %d", extra())
	}
	tariff.ExtraDevices = 0 // «Без доп. устройств»
	resolve(tariff, model.PaymentConfirmed)
	if extra() != 0 {
		t.Fatalf("drop: %d", extra())
	}
}

func TestResolveSeesCurrentExtra(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	first, _, err := s.CreatePayment(ctx, devicesPay(1, 2), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolvePayment(ctx, first.ID, model.PaymentConfirmed, time.Now(), noApply); err != nil {
		t.Fatal(err)
	}
	second, _, err := s.CreatePayment(ctx, devicesPay(1, 1), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	var seen int
	if _, err = s.ResolvePayment(ctx, second.ID, model.PaymentConfirmed, time.Now(), func(x model.Payment) ([]model.OutboxMessage, error) {
		seen = x.UserExtra
		return nil, nil
	}); err != nil || seen != 2 {
		t.Fatalf("seen %d err %v", seen, err)
	}
}

// Докупка устройств не делает юзера «не новым» и не сбрасывает счётчик неоплаченных авансов.
func TestDevicesPaymentIsNotATariff(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	adv, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantAdvance(ctx, adv.ID, func(model.Payment) {}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolvePayment(ctx, adv.ID, model.PaymentDeclined, time.Now(), noApply); err != nil {
		t.Fatal(err)
	}
	dev, _, err := s.CreatePayment(ctx, devicesPay(1, 1), noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolvePayment(ctx, dev.ID, model.PaymentConfirmed, time.Now(), noApply); err != nil {
		t.Fatal(err)
	}
	st, err := s.UserStats(ctx, 1, 0)
	if err != nil || st.UnpaidAdvances != 1 {
		t.Fatalf("stats %+v %v", st, err)
	}
	if has, err := s.HasConfirmed(ctx, 1); err != nil || has {
		t.Fatalf("HasConfirmed = %v %v", has, err)
	}
	u, err := s.UserByID(ctx, 1)
	if err != nil || u.TariffMonths != nil || u.ExtraDevices != 1 {
		t.Fatalf("user %+v %v", u, err)
	}
}

// Двойное подтверждение докупки (бот и панель): лимит пишется один раз, устройства засчитываются один раз.
func TestDevicesConfirmIdempotentUnderRace(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
	dev := devicesPay(1, 2)
	dev.PrevExpireAt = prev
	p, _, err := s.CreatePayment(ctx, dev, noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	panel := &statePanel{expire: prev, fast: true}
	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, admin := range []int64{0, 99} {
		wg.Go(func() {
			<-start
			_, errs[i] = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID, Approve: true, At: now, AdminID: admin, DevicesBase: 5})
		})
	}
	close(start)
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, ErrNotPending):
			t.Fatalf("errs = %v", errs)
		}
	}
	extra, _ := s.ExtraDevices(ctx, 1)
	if won != 1 || extra != 2 || len(panel.limits) != 1 || panel.limits[0] != 7 || len(panel.writes) != 0 {
		t.Fatalf("won %d extra %d limits %v term writes %v", won, extra, panel.limits, panel.writes)
	}
}

// Докупка не двигает срок: выдача дней рядом даёт ровно +N дней, а лимит пишется отдельно.
func TestDevicesConfirmRaceWithGrantKeepsTerm(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	prev := now.Add(48 * time.Hour).Truncate(time.Microsecond)
	dev := devicesPay(1, 1)
	dev.PrevExpireAt = prev
	p, _, err := s.CreatePayment(ctx, dev, noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	panel := &statePanel{expire: prev}
	var wg sync.WaitGroup
	var gerr, cerr error
	wg.Go(func() {
		_, gerr = payflow.GrantDays(ctx, s, panel, payflow.GrantRequest{TelegramID: 1, Days: 7, At: now})
	})
	wg.Go(func() {
		time.Sleep(20 * time.Millisecond)
		_, cerr = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID, Approve: true, At: now, DevicesBase: 5})
	})
	wg.Wait()
	if gerr != nil || cerr != nil {
		t.Fatalf("grant=%v confirm=%v", gerr, cerr)
	}
	if want := prev.Add(7 * 24 * time.Hour); !panel.cur().Equal(want) {
		t.Fatalf("term = %v, want %v", panel.cur(), want)
	}
	if len(panel.limits) != 1 || panel.limits[0] != 6 || len(panel.writes) != 1 {
		t.Fatalf("limits %v term writes %v", panel.limits, panel.writes)
	}
}

// Подписка: лимит пишется только при изменении числа устройств; отклонение панель по устройствам не трогает.
func TestTariffConfirmWritesLimitOnlyOnChange(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	panel := &statePanel{expire: now.Add(24 * time.Hour), fast: true}
	run := func(extra int, approve bool) {
		t.Helper()
		pay := newPay(1, now.Add(24*time.Hour))
		pay.ExtraDevices = extra
		p, _, err := s.CreatePayment(ctx, pay, noRollback, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = payflow.Resolve(ctx, s, panel, payflow.Decision{ID: p.ID, Approve: approve, At: now, DevicesBase: 5}); err != nil {
			t.Fatal(err)
		}
	}
	run(0, true) // число не менялось
	if len(panel.limits) != 0 {
		t.Fatalf("limits %v", panel.limits)
	}
	run(2, false) // отклонён
	if len(panel.limits) != 0 {
		t.Fatalf("limits after decline %v", panel.limits)
	}
	run(2, true)
	run(2, true) // продление с тем же числом
	if len(panel.limits) != 1 || panel.limits[0] != 7 {
		t.Fatalf("limits %v", panel.limits)
	}
	run(0, true) // «Без доп. устройств»
	if len(panel.limits) != 2 || panel.limits[1] != 5 {
		t.Fatalf("limits %v", panel.limits)
	}
	if n, _ := s.ExtraDevices(ctx, 1); n != 0 {
		t.Fatalf("extra %d", n)
	}
}

// Сбой панели при записи лимита откатывает транзакцию: ни статуса, ни устройств; повтор проходит.
func TestDevicesConfirmPanelFailureRollsBack(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	dev := devicesPay(1, 3)
	dev.PrevExpireAt = time.Now()
	p, _, err := s.CreatePayment(ctx, dev, noRollback, nil)
	if err != nil {
		t.Fatal(err)
	}
	panel := &racePanel{err: errors.New("down")}
	d := payflow.Decision{ID: p.ID, Approve: true, At: time.Now(), DevicesBase: 5}
	if _, err = payflow.Resolve(ctx, s, panel, d); !errors.Is(err, payflow.ErrPanel) {
		t.Fatalf("err = %v", err)
	}
	if n, _ := s.ExtraDevices(ctx, 1); n != 0 {
		t.Fatalf("extra %d after failure", n)
	}
	panel.err = nil
	if _, err = payflow.Resolve(ctx, s, panel, d); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ExtraDevices(ctx, 1); n != 3 || len(panel.limits) != 1 || panel.limits[0] != 8 {
		t.Fatalf("extra %d limits %v", n, panel.limits)
	}
	items, _ := s.OutboxBatch(ctx, 10)
	if len(items) != 1 || items[0].TextKey != "payment.devices_confirmed" || items[0].Devices != 3 {
		t.Fatalf("outbox %+v", items)
	}
}
