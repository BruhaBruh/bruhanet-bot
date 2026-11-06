package routes_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
)

func (e *env) menuHasTrial(t *testing.T) bool {
	t.Helper()
	e.process(t, tgtest.Message(userID, "/start"))
	return strings.Contains(e.last(t, "sendMessage").Params.Get("reply_markup"), "Пробный период")
}

// startTrial жмёт «Пробный период» и «Да».
func (e *env) startTrial(t *testing.T) {
	t.Helper()
	e.process(t, tgtest.Callback(userID, "trial"))
	e.process(t, tgtest.Callback(userID, "trialy"))
}

func TestTrialFlow(t *testing.T) {
	e := newEnv(t)
	if !e.menuHasTrial(t) {
		t.Fatal("новому юзеру нет кнопки пробного")
	}
	e.process(t, tgtest.Callback(userID, "trial"))
	if text := e.text(t); !strings.Contains(text, "<b>3</b>") || !strings.Contains(text, "<b>2</b>") {
		t.Fatalf("confirm: %s", text)
	}
	if e.rw.ExpireAt(userID) != (time.Time{}) {
		t.Fatal("экран подтверждения уже выдал срок")
	}

	e.process(t, tgtest.Callback(userID, "trialy"))
	if got, want := e.rw.ExpireAt(userID), now.Add(3*day); !got.Equal(want) {
		t.Fatalf("срок %v, want %v", got, want)
	}
	if e.rw.DeviceLimit(userID) != 2 {
		t.Fatalf("лимит %d", e.rw.DeviceLimit(userID))
	}
	if _, ok := e.pay.TrialUsedAt(userID); !ok {
		t.Fatal("trial_used_at не записан")
	}
	if g := e.pay.Grants(); len(g) != 1 || g[0].Days != 3 || g[0].Source != "trial" {
		t.Fatalf("grants %+v", g)
	}
	out := e.pay.Outbox()
	if len(out) != 1 || out[0].TextKey != "trial.started" || out[0].Days != 3 || out[0].Devices != 2 || !out[0].ExpireAt.Equal(now.Add(3*day)) || out[0].ChatID != userID {
		t.Fatalf("outbox %+v", out)
	}
	if e.menuHasTrial(t) {
		t.Fatal("кнопка осталась после выдачи")
	}

	// двойное «Да» (или повтор со старого экрана): срок не растёт, вторая запись не появляется
	before := len(e.rw.Updates())
	e.process(t, tgtest.Callback(userID, "trialy"))
	if len(e.rw.Updates()) != before || len(e.pay.Grants()) != 1 || len(e.pay.Outbox()) != 1 {
		t.Fatal("повторная выдача")
	}
	if text := e.text(t); !strings.Contains(text, "недоступен") {
		t.Fatalf("text: %s", text)
	}
}

func TestTrialEligibility(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, e *env)
		ok    bool
	}{
		"new":              {func(*testing.T, *env) {}, true},
		"in panel expired": {func(_ *testing.T, e *env) { e.rw.AddUser(userID, now.Add(-time.Hour)) }, true},
		"blocked":          {func(_ *testing.T, e *env) { e.pay.SetBlocked(userID, true) }, false},
		"already used":     {func(_ *testing.T, e *env) { e.pay.SetTrialUsed(userID, now.Add(-90*day)) }, false},
		"active panel sub": {func(_ *testing.T, e *env) { e.rw.AddUser(userID, now.Add(5*day)) }, false},
		"old customer outside db": {func(_ *testing.T, e *env) {
			e.rw.AddUserCreatedAt(userID, now.Add(-day), now.Add(-400*day))
		}, false},
		"paid before": {func(t *testing.T, e *env) {
			e.rw.AddUser(userID, now)
			e.process(t, tgtest.Callback(userID, "buy:1"))
			e.process(t, tgtest.Photo(userID))
			e.process(t, confirmPress(e.pending(t, userID).ID))
			e.rw.AddUser(userID, now.Add(-day)) // подписка давно кончилась
		}, false},
		"declined payments below the limit": {func(t *testing.T, e *env) {
			for range 2 {
				e.process(t, tgtest.Callback(userID, "buy:1"))
				e.process(t, tgtest.Photo(userID))
				e.process(t, declinePress(e.pending(t, userID).ID))
			}
		}, true},
		"advance limit reached": {func(t *testing.T, e *env) {
			for range 3 {
				e.process(t, tgtest.Callback(userID, "buy:1"))
				e.process(t, tgtest.Photo(userID))
				e.process(t, declinePress(e.pending(t, userID).ID))
			}
		}, false},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tc.setup(t, e)
			if got := e.menuHasTrial(t); got != tc.ok {
				t.Fatalf("кнопка в меню = %v, want %v", got, tc.ok)
			}
			before := len(e.rw.Updates())
			e.startTrial(t)
			_, used := e.pay.TrialUsedAt(userID)
			// «Нужно ли что-то выдавать» отличается от кнопки: пробный записан только у подходящих (и не был записан до)
			granted := len(e.pay.Grants()) == 1
			if name == "already used" {
				if granted || len(e.rw.Updates()) != before {
					t.Fatal("повторная выдача")
				}
				return
			}
			if granted != tc.ok || (tc.ok && !used) {
				t.Fatalf("выдан = %v, want %v", granted, tc.ok)
			}
			if !tc.ok && len(e.rw.Updates()) != before {
				t.Fatalf("панель тронута отказом: %v", e.rw.Updates())
			}
			if !tc.ok && len(e.pay.Outbox()) > 0 && e.pay.Outbox()[0].TextKey == "trial.started" {
				t.Fatal("уведомление о выдаче при отказе")
			}
		})
	}
}

func TestTrialDisabledAndSettings(t *testing.T) {
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{"trialDays": []byte(`0`)})
	if e.menuHasTrial(t) {
		t.Fatal("кнопка при trialDays=0")
	}
	e.startTrial(t) // подделанный callback
	if len(e.pay.Grants()) != 0 || e.rw.ExpireAt(userID) != (time.Time{}) {
		t.Fatal("выдан при выключенном")
	}

	e.src.SetSettings(map[string][]byte{"trialDays": []byte(`7`), "trialDevices": []byte(`4`)})
	e.startTrial(t)
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(7 * day)) {
		t.Fatalf("срок %v", got)
	}
	if e.rw.DeviceLimit(userID) != 4 {
		t.Fatalf("лимит %d", e.rw.DeviceLimit(userID))
	}
}

func TestTrialPanelFailureRollsBack(t *testing.T) {
	e := newEnv(t)
	e.rw.AddUser(userID, now.Add(-time.Hour))
	e.rw.FailUpdateWith(errors.New("panel down"))
	e.startTrial(t)
	if _, ok := e.pay.TrialUsedAt(userID); ok || len(e.pay.Grants()) != 0 || len(e.pay.Outbox()) != 0 {
		t.Fatal("сбой панели оставил следы")
	}
	if e.rw.ExpireAt(userID).After(now) {
		t.Fatal("срок записан")
	}
	e.rw.FailUpdateWith(nil)
	e.startTrial(t) // повтор считает ту же дату
	if got := e.rw.ExpireAt(userID); !got.Equal(now.Add(3 * day)) {
		t.Fatalf("срок %v", got)
	}
}

func TestTrialCommitFailureRestoresPanel(t *testing.T) {
	e := newEnv(t)
	old := now.Add(-time.Hour)
	e.rw.AddUser(userID, old)
	e.pay.FailTrialCommitOnce()
	e.startTrial(t)
	if _, ok := e.pay.TrialUsedAt(userID); ok {
		t.Fatal("trial записан")
	}
	if got := e.rw.ExpireAt(userID); !got.Equal(old) {
		t.Fatalf("срок не возвращён: %v", got)
	}
	if w := e.rw.DeviceLimitWrites(); len(w) != 2 || w[0].Limit != 2 || w[1].Limit != 5 {
		t.Fatalf("лимит не возвращён к базе: %+v", w)
	}
}

// Пробный — один неоплаченный аванс: с лимитом 2 остаётся один аванс; подтверждённая оплата сбрасывает счёт
// и пишет в панель base+extra вместо пробного лимита, один раз.
func TestTrialCountsAsAdvanceAndConvertsToPaid(t *testing.T) {
	e := newEnv(t)
	e.src.SetSettings(map[string][]byte{"advanceLimit": []byte(`2`)})
	e.startTrial(t)
	st, _ := e.pay.UserStats(t.Context(), userID, 0)
	if st.UnpaidAdvances != 1 {
		t.Fatalf("после выдачи неоплаченных авансов %d", st.UnpaidAdvances)
	}

	_, advanced := e.round(t) // второй аванс — последний допустимый
	if !advanced {
		t.Fatal("аванс при 1 из 2")
	}
	e.process(t, declinePress(e.pending(t, userID).ID))
	if _, advanced = e.round(t); advanced {
		t.Fatal("аванс сверх лимита: пробный не учтён")
	}

	// срок от конца пробного, лимит устройств пишется
	prev := e.pending(t, userID).PrevExpireAt
	if want := now.Add(3 * day); !prev.Equal(want) {
		t.Fatalf("база продления %v, want %v", prev, want)
	}
	id := e.pending(t, userID).ID
	limits := len(e.rw.DeviceLimitWrites())
	e.process(t, confirmPress(id))
	w := e.rw.DeviceLimitWrites()
	if len(w) != limits+1 || w[len(w)-1].Limit != 5 || e.rw.DeviceLimit(userID) != 5 {
		t.Fatalf("конвертация не записала base+extra: %+v", w)
	}
	if st, _ = e.pay.UserStats(t.Context(), userID, 0); st.UnpaidAdvances != 0 {
		t.Fatalf("после оплаты неоплаченных авансов %d", st.UnpaidAdvances)
	}

	// следующая оплата лимит не трогает
	e.rw.AddUser(userID, now.Add(-time.Hour)) // срок кончился
	e.process(t, tgtest.Callback(userID, "buy:1"))
	e.process(t, tgtest.Photo(userID))
	e.process(t, confirmPress(e.pending(t, userID).ID))
	if got := len(e.rw.DeviceLimitWrites()); got != len(w) {
		t.Fatalf("лишняя запись лимита: %+v", e.rw.DeviceLimitWrites())
	}
}

// Пробный при открытой оплате двигает базу pending-платежа: confirm/decline не затрут дни.
func TestTrialShiftsPendingBase(t *testing.T) {
	e := newEnv(t)
	e.process(t, tgtest.Callback(userID, "buy:6"))
	if p := e.pending(t, userID); !p.PrevExpireAt.Equal(now) {
		t.Fatalf("prev %v", p.PrevExpireAt)
	}
	e.startTrial(t)
	if p := e.pending(t, userID); !p.PrevExpireAt.Equal(now.Add(3 * day)) {
		t.Fatalf("база pending не сдвинута: %v", p.PrevExpireAt)
	}
	_ = model.PaymentPending
}
