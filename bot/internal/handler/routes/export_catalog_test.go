package routes

import (
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
)

// CheckCatalog — каждое route-действие каталога есть в routeData, привязанное — в argKey, шаблон — в templateBlock,
// каждое условие считается (не «unknown»).
func CheckCatalog(t *testing.T) {
	t.Helper()
	for _, a := range texts.Actions() {
		kind, name, _ := strings.Cut(a.ID, ":")
		ok := true
		switch a.Kind {
		case texts.ActionRoute:
			_, ok = routeData[name]
		case texts.ActionFlow, texts.ActionCopy:
			_, ok = argKey[a.ID]
		case texts.ActionTemplate:
			_, ok = templateBlock[name]
		case texts.ActionLink, texts.ActionURLKind:
			_, bound := argKey[a.ID] // «link:login» питает экран входа, как flow/copy
			ok = a.ID == "link:support" || bound
		}
		if !ok {
			t.Errorf("action %s (%s) has no implementation", a.ID, kind)
		}
	}
}

// Условия каталога: каждое считается из настроек и состояния юзера; неизвестное — ложь.
func TestConditionEvaluators(t *testing.T) {
	set := settings.Settings{DevicePrice: 60, ReferralDays: 7, Prices: map[int]int{1: 100}}
	p := content.New(&tgtest.Source{}, set, time.Hour, nil)
	e := &condEnv{ctx: t.Context(), c: p, panel: func() bool { return true }, trial: func() bool { return false }}
	want := map[string]bool{"in_panel": true, "trial_available": false, "devices_sale_on": true, "gifts_on": true, "referral_on": true}
	for _, c := range texts.Conditions() {
		w, ok := want[c.ID]
		if !ok {
			t.Errorf("condition %s has no expectation: add an evaluator test", c.ID)
			continue
		}
		if got := e.eval(c.ID); got != w {
			t.Errorf("%s = %v, want %v", c.ID, got, w)
		}
	}
	off := &condEnv{ctx: t.Context(), c: content.New(&tgtest.Source{}, settings.Settings{}, time.Hour, nil)}
	for _, c := range texts.Conditions() {
		if off.eval(c.ID) {
			t.Errorf("%s true with nothing configured", c.ID)
		}
	}
	if e.eval("nope") {
		t.Error("unknown condition must be false")
	}
}
