package observability

import "testing"

func TestPaymentLabel(t *testing.T) {
	for _, c := range []struct {
		kind   string
		months int
		want   string
	}{{"tariff", 6, "6"}, {"", 1, "1"}, {"devices", 0, "devices"}, {"gift", 3, "gift"}} {
		if got := PaymentLabel(c.kind, c.months); got != c.want {
			t.Errorf("%q/%d = %q, want %q", c.kind, c.months, got, c.want)
		}
	}
}
