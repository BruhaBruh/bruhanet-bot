package routes_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

// метрики глобальные, поэтому сверяем только приращения.
func payments(stage string) float64 {
	return testutil.ToFloat64(observability.Payments.WithLabelValues(stage, "6"))
}

func rubles() float64 {
	return testutil.ToFloat64(observability.PaymentsConfirmedRubles.WithLabelValues("6"))
}

func wantDelta(t *testing.T, name string, before, after, want float64) {
	t.Helper()
	if after-before != want {
		t.Fatalf("%s delta = %v, want %v", name, after-before, want)
	}
}

func TestPaymentMetricsFlow(t *testing.T) {
	e := newEnv(t)
	started, photo, confirmed, declined, rub := payments("started"), payments("photo"), payments("confirmed"), payments("declined"), rubles()

	e.process(t, tgtest.Callback(userID, "buy:6"))
	wantDelta(t, "started", started, payments("started"), 1)

	e.process(t, tgtest.Photo(userID))
	wantDelta(t, "photo", photo, payments("photo"), 1)

	adminMsg := e.payment(t, userID).AdminMessageID
	e.process(t, adminPress("adm_ok:42", adminMsg))
	wantDelta(t, "confirmed", confirmed, payments("confirmed"), 1)
	wantDelta(t, "rubles", rub, rubles(), 800)

	e.process(t, adminPress("adm_ok:42", adminMsg)) // повторный тап
	wantDelta(t, "confirmed after double tap", confirmed, payments("confirmed"), 1)
	wantDelta(t, "rubles after double tap", rub, rubles(), 800)
	wantDelta(t, "declined", declined, payments("declined"), 0)
}

func TestPaymentMetricsDecline(t *testing.T) {
	e := newEnv(t)
	inReview(t, e, now)
	declined, confirmed, rub := payments("declined"), payments("confirmed"), rubles()

	e.process(t, adminPress("adm_no:42", adminMessageID))

	wantDelta(t, "declined", declined, payments("declined"), 1)
	wantDelta(t, "confirmed", confirmed, payments("confirmed"), 0)
	wantDelta(t, "rubles", rub, rubles(), 0)
}
