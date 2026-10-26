package callback_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/callback"
)

func TestRouteDataParse(t *testing.T) {
	price := callback.New[int]("price")
	if price.Name() != "price" {
		t.Fatalf("Name = %q", price.Name())
	}
	data := price.Data(6)
	if data != "price:6" {
		t.Fatalf("Data = %q", data)
	}
	got, err := price.Parse(data)
	if err != nil || got != 6 {
		t.Fatalf("Parse = %d, %v", got, err)
	}
}

func TestRouteInt64(t *testing.T) {
	confirm := callback.New[int64]("adm_ok")
	const id int64 = 9_000_000_000
	got, err := confirm.Parse(confirm.Data(id))
	if err != nil || got != id {
		t.Fatalf("Parse = %d, %v", got, err)
	}
}

func TestParseErrors(t *testing.T) {
	price := callback.New[int]("price")
	for _, data := range []string{"", "price", "price:", "price:abc", "price:6:7", "pay:6", "prices:6"} {
		t.Run(data, func(t *testing.T) {
			if _, err := price.Parse(data); !errors.Is(err, callback.ErrInvalidData) {
				t.Fatalf("ожидался ErrInvalidData, получено %v", err)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	for _, name := range []string{"pay", "adm_ok", "a1", strings.Repeat("a", 40)} {
		if err := callback.ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"", "Pay", "a:b", "a b", "оплата", strings.Repeat("a", 41)} {
		if err := callback.ValidateName(name); !errors.Is(err, callback.ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, ожидался ErrInvalidName", name, err)
		}
	}
}

func TestNewPanicsOnInvalidName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("ожидалась паника")
		}
	}()
	callback.New[int]("Pay")
}

func TestMaxDataFitsTelegramLimit(t *testing.T) {
	r := callback.New[int64](strings.Repeat("a", 40))
	if n := len(r.Data(math.MinInt64)); n > 64 {
		t.Fatalf("data = %d байт, лимит 64", n)
	}
}
