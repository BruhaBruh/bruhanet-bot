package store

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/promo"
)

func ptr[T any](v T) *T { return &v }

func mustPromo(t *testing.T, s *Store, p model.Promo) model.Promo {
	t.Helper()
	created, err := s.CreatePromo(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestCreatePromoAndByCode(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	exp := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	p := mustPromo(t, s, model.Promo{
		Code: " summer25 ", Kind: promo.KindPercent, Value: 25,
		MaxActivations: ptr(3), ExpiresAt: &exp, OnlyNew: true, TelegramID: ptr[int64](42),
	})
	if p.ID == 0 || p.Code != "SUMMER25" || p.CreatedAt.IsZero() {
		t.Fatalf("created %+v", p)
	}
	if _, err := s.CreatePromo(ctx, model.Promo{Code: "Summer25", Kind: promo.KindDays, Value: 1}); !errors.Is(err, ErrPromoExists) {
		t.Fatalf("duplicate: want ErrPromoExists, got %v", err)
	}

	// Review Focus 5: пробелы и нижний регистр.
	got, err := s.PromoByCode(ctx, "  sUmmer25\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID || *got.MaxActivations != 3 || !got.ExpiresAt.Equal(exp) || !got.OnlyNew || *got.TelegramID != 42 || got.Kind != promo.KindPercent {
		t.Fatalf("by code %+v", got)
	}
	if _, err = s.PromoByCode(ctx, "NOPE"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	byID, err := s.PromoByID(ctx, p.ID)
	if err != nil || byID.Code != "SUMMER25" {
		t.Fatalf("by id %+v err=%v", byID, err)
	}
	if _, err = s.PromoByID(ctx, p.ID+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListDisablePromos(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now()
	used := mustPromo(t, s, model.Promo{Code: "USED", Kind: promo.KindDays, Value: 1, MaxActivations: ptr(2)})
	full := mustPromo(t, s, model.Promo{Code: "FULL", Kind: promo.KindDays, Value: 1, MaxActivations: ptr(1)})
	old := mustPromo(t, s, model.Promo{Code: "OLD", Kind: promo.KindDays, Value: 1, ExpiresAt: ptr(now.Add(-time.Hour))})
	off := mustPromo(t, s, model.Promo{Code: "OFF", Kind: promo.KindDays, Value: 1})
	for _, id := range []int64{used.ID, full.ID} {
		if err := s.ActivateDaysPromo(ctx, id, 1, now, func(model.Promo) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DisablePromo(ctx, off.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DisablePromo(ctx, off.ID+100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	all, err := s.ListPromos(ctx, false, now)
	if err != nil {
		t.Fatal(err)
	}
	usedBy := map[string]int{}
	for _, p := range all {
		usedBy[p.Code] = p.Used
	}
	if len(all) != 4 || all[0].ID != off.ID || !all[0].Disabled || usedBy["USED"] != 1 || usedBy["FULL"] != 1 || usedBy["OLD"] != 0 {
		t.Fatalf("all = %+v", all)
	}
	active, err := s.ListPromos(ctx, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != used.ID {
		t.Fatalf("active = %+v (old=%d)", active, old.ID)
	}
}

func TestActivateDaysPromo(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now()
	p := mustPromo(t, s, model.Promo{Code: "DAYS", Kind: promo.KindDays, Value: 5})
	if _, _, err := s.CreatePayment(ctx, model.Payment{TelegramID: 7, Username: "vasya", Month: 1, Price: 1, PrevExpireAt: now}, noRollback); err != nil {
		t.Fatal(err)
	}

	// Review Focus 4: ошибка панели — активации нет, можно снова.
	boom := errors.New("panel down")
	if err := s.ActivateDaysPromo(ctx, p.ID, 7, now, func(model.Promo) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if used, already, err := s.PromoUsage(ctx, p.ID, 7); err != nil || used != 0 || already {
		t.Fatalf("after failed apply used=%d already=%v err=%v", used, already, err)
	}

	var got model.Promo
	if err := s.ActivateDaysPromo(ctx, p.ID, 7, now, func(p model.Promo) error { got = p; return nil }); err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID || got.Value != 5 {
		t.Fatalf("apply got %+v", got)
	}
	if used, already, err := s.PromoUsage(ctx, p.ID, 7); err != nil || used != 1 || !already {
		t.Fatalf("used=%d already=%v err=%v", used, already, err)
	}
	if err := s.ActivateDaysPromo(ctx, p.ID, 7, now, func(model.Promo) error { t.Fatal("apply on reuse"); return nil }); !errors.Is(err, promo.ErrAlreadyUsed) {
		t.Fatalf("reuse: want ErrAlreadyUsed, got %v", err)
	}
	acts, err := s.PromoActivations(ctx, p.ID)
	if err != nil || len(acts) != 1 || acts[0].TelegramID != 7 || acts[0].Username != "vasya" || acts[0].PaymentID != nil {
		t.Fatalf("activations %+v err=%v", acts, err)
	}

	disc := mustPromo(t, s, model.Promo{Code: "DISC", Kind: promo.KindPercent, Value: 5})
	if err = s.ActivateDaysPromo(ctx, disc.ID, 7, now, func(model.Promo) error { return nil }); !errors.Is(err, promo.ErrWrongPlace) {
		t.Fatalf("discount in menu: want ErrWrongPlace, got %v", err)
	}
	if err = s.ActivateDaysPromo(ctx, disc.ID+100, 7, now, func(model.Promo) error { return nil }); !errors.Is(err, promo.ErrNotFound) {
		t.Fatalf("want promo.ErrNotFound, got %v", err)
	}
}

// Review Focus 1: два юзера берут последнюю активацию — успешен ровно один.
func TestActivateDaysPromoConcurrent(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p := mustPromo(t, s, model.Promo{Code: "LAST", Kind: promo.KindDays, Value: 1, MaxActivations: ptr(1)})

	var applied atomic.Int32
	errs := make([]error, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			errs[i] = s.ActivateDaysPromo(ctx, p.ID, int64(100+i), time.Now(), func(model.Promo) error {
				applied.Add(1)
				<-release // держим tx, пока второй не упрётся в FOR UPDATE
				return nil
			})
		})
	}
	for applied.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()

	if applied.Load() != 1 {
		t.Fatalf("apply calls = %d", applied.Load())
	}
	var ok, exhausted int
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, promo.ErrExhausted):
			exhausted++
		default:
			t.Fatalf("unexpected: %v", e)
		}
	}
	if ok != 1 || exhausted != 1 {
		t.Fatalf("ok=%d exhausted=%d", ok, exhausted)
	}
}

func TestHasConfirmed(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	if has, err := s.HasConfirmed(ctx, 1); err != nil || has {
		t.Fatalf("before: %v %v", has, err)
	}
	if _, err = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), okApply); err != nil {
		t.Fatal(err)
	}
	if has, err := s.HasConfirmed(ctx, 1); err != nil || !has {
		t.Fatalf("after: %v %v", has, err)
	}
}

func TestResolvePaymentPromoActivation(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	pr := mustPromo(t, s, model.Promo{Code: "SALE", Kind: promo.KindPercent, Value: 25, MaxActivations: ptr(1)})

	pay := func(tg int64) model.Payment {
		p := newPay(tg, time.Now())
		p.Price, p.PromoID, p.BasePrice = 112, &pr.ID, ptr(150)
		created, _, err := s.CreatePayment(ctx, p, noRollback)
		if err != nil {
			t.Fatal(err)
		}
		if created.PromoID == nil || *created.PromoID != pr.ID || created.BasePrice == nil || *created.BasePrice != 150 {
			t.Fatalf("created %+v", created)
		}
		return created
	}

	declined := pay(1)
	if _, err := s.ResolvePayment(ctx, declined.ID, model.PaymentDeclined, time.Now(), okApply); err != nil {
		t.Fatal(err)
	}
	if used, _, err := s.PromoUsage(ctx, pr.ID, 1); err != nil || used != 0 {
		t.Fatalf("declined: used=%d err=%v", used, err)
	}

	// Два подтверждения при лимите 1: оба засчитываются (лимит кончился между вводом и подтверждением).
	for _, tg := range []int64{1, 2} {
		p := pay(tg)
		if _, err := s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), okApply); err != nil {
			t.Fatal(err)
		}
	}
	acts, err := s.PromoActivations(ctx, pr.ID)
	if err != nil || len(acts) != 2 || acts[0].PaymentID == nil {
		t.Fatalf("activations %+v err=%v", acts, err)
	}
	// Повторное подтверждение тем же юзером — без ошибки и без второй активации.
	p := pay(1)
	if _, err = s.ResolvePayment(ctx, p.ID, model.PaymentConfirmed, time.Now(), okApply); err != nil {
		t.Fatal(err)
	}
	if used, already, err := s.PromoUsage(ctx, pr.ID, 1); err != nil || used != 2 || !already {
		t.Fatalf("used=%d already=%v err=%v", used, already, err)
	}

	// Платёж без промокода читается с nil-полями.
	plain, _, err := s.CreatePayment(ctx, newPay(3, time.Now()), noRollback)
	if err != nil || plain.PromoID != nil || plain.BasePrice != nil {
		t.Fatalf("plain %+v err=%v", plain, err)
	}
}

// Цена 0: активация пишется до apply и без ON CONFLICT — повтор отклоняется в БД, панель не трогается.
func TestResolveFreePromoPaymentRejectsRepeat(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	pr := mustPromo(t, s, model.Promo{Code: "FREE", Kind: promo.KindPercent, Value: 100})
	free := func() model.Payment {
		p := newPay(1, time.Now())
		p.Price, p.PromoID, p.BasePrice = 0, &pr.ID, ptr(150)
		created, _, err := s.CreatePayment(ctx, p, noRollback)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	if _, err := s.ResolvePayment(ctx, free().ID, model.PaymentConfirmed, time.Now(), okApply); err != nil {
		t.Fatal(err)
	}
	second := free()
	applied := false
	_, err := s.ResolvePayment(ctx, second.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) (model.OutboxMessage, error) {
		applied = true
		return okApply(second)
	})
	if !errors.Is(err, promo.ErrAlreadyUsed) || applied {
		t.Fatalf("err = %v, applied = %v", err, applied)
	}
	if got, err := s.PaymentByID(ctx, second.ID); err != nil || got.Status != model.PaymentPending {
		t.Fatalf("second = %+v, %v", got, err)
	}
	if used, _, err := s.PromoUsage(ctx, pr.ID, 1); err != nil || used != 1 {
		t.Fatalf("used = %d, %v", used, err)
	}
	// ошибка apply откатывает и активацию
	other := newPay(2, time.Now())
	other.Price, other.PromoID = 0, &pr.ID
	o, _, err := s.CreatePayment(ctx, other, noRollback)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolvePayment(ctx, o.ID, model.PaymentConfirmed, time.Now(), func(model.Payment) (model.OutboxMessage, error) {
		return model.OutboxMessage{}, errors.New("panel down")
	}); err == nil {
		t.Fatal("ожидалась ошибка apply")
	}
	if _, already, err := s.PromoUsage(ctx, pr.ID, 2); err != nil || already {
		t.Fatalf("активация после отката: %v %v", already, err)
	}
}

func TestCancelPayment(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	p, _, err := s.CreatePayment(ctx, newPay(1, time.Now()), noRollback)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelPayment(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.PaymentByID(ctx, p.ID); got.Status != model.PaymentCancelled || got.ResolvedAt == nil {
		t.Fatalf("payment = %+v", got)
	}
	if err = s.CancelPayment(ctx, p.ID); !errors.Is(err, ErrNotPending) {
		t.Fatalf("повтор: %v", err)
	}
	if err = s.CancelPayment(ctx, 999999); !errors.Is(err, ErrNotPending) {
		t.Fatalf("нет платежа: %v", err)
	}
}

// Последняя активация кода 100%: два юзера подтверждают 0 ₽ одновременно — успешен ровно один.
func TestResolveFreePromoConcurrentLimit(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	pr := mustPromo(t, s, model.Promo{Code: "FREELAST", Kind: promo.KindPercent, Value: 100, MaxActivations: ptr(1)})
	ids := make([]int64, 2)
	for i := range ids {
		p := newPay(int64(200+i), time.Now())
		p.Price, p.PromoID, p.BasePrice = 0, &pr.ID, ptr(150)
		created, _, err := s.CreatePayment(ctx, p, noRollback)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = created.ID
	}

	var applied atomic.Int32
	errs := make([]error, 2)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = s.ResolvePayment(ctx, ids[i], model.PaymentConfirmed, time.Now(), func(p model.Payment) (model.OutboxMessage, error) {
				applied.Add(1)
				<-release // держим tx, пока второй не упрётся в блокировку кода
				return okApply(p)
			})
		})
	}
	for applied.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	close(release)
	wg.Wait()

	var ok, exhausted int
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, promo.ErrExhausted):
			exhausted++
		default:
			t.Fatalf("unexpected: %v", e)
		}
	}
	if ok != 1 || exhausted != 1 || applied.Load() != 1 {
		t.Fatalf("ok=%d exhausted=%d applied=%d", ok, exhausted, applied.Load())
	}
}

// Дни при pending-платеже сдвигают его базу: подтверждение и отклонение учитывают бонус.
func TestActivateDaysPromoShiftsPending(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Microsecond)
	future := now.Add(10 * 24 * time.Hour)
	pr := mustPromo(t, s, model.Promo{Code: "PLUS5", Kind: promo.KindDays, Value: 5})
	bonus := 5 * 24 * time.Hour

	for _, c := range []struct {
		tg     int64
		prev   time.Time
		status model.PaymentStatus
		want   time.Time
	}{
		{1, future, model.PaymentConfirmed, future.Add(bonus)},
		{2, future, model.PaymentDeclined, future.Add(bonus)},
		{3, now.Add(-48 * time.Hour), model.PaymentConfirmed, now.Add(bonus)}, // срок в прошлом — от now
	} {
		p, _, err := s.CreatePayment(ctx, newPay(c.tg, c.prev), noRollback)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ActivateDaysPromo(ctx, pr.ID, c.tg, now, func(model.Promo) error { return nil }); err != nil {
			t.Fatal(err)
		}
		var got time.Time
		if _, err = s.ResolvePayment(ctx, p.ID, c.status, now, func(x model.Payment) (model.OutboxMessage, error) {
			got = x.PrevExpireAt
			return okApply(x)
		}); err != nil {
			t.Fatal(err)
		}
		if !got.Equal(c.want) {
			t.Fatalf("tg %d: prev = %v, want %v", c.tg, got, c.want)
		}
	}
}
