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
				t.Fatalf("want ErrInvalidData, got %v", err)
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
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestNewPanicsOnInvalidName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("panic expected")
		}
	}()
	callback.New[int]("Pay")
}

func TestMaxDataFitsTelegramLimit(t *testing.T) {
	r := callback.New[int64](strings.Repeat("a", 40))
	if n := len(r.Data(math.MinInt64)); n > 64 {
		t.Fatalf("data = %d bytes, limit 64", n)
	}
}

func TestRoute2DataParse(t *testing.T) {
	buy := callback.New2[int, int64]("buyp")
	if buy.Name() != "buyp" {
		t.Fatalf("Name = %q", buy.Name())
	}
	data := buy.Data(6, 9_000_000_000)
	if data != "buyp:6:9000000000" {
		t.Fatalf("Data = %q", data)
	}
	a, b, err := buy.Parse(data)
	if err != nil || a != 6 || b != 9_000_000_000 {
		t.Fatalf("Parse = %d, %d, %v", a, b, err)
	}
}

func TestRoute2ParseErrors(t *testing.T) {
	buy := callback.New2[int, int64]("buyp")
	for _, data := range []string{"", "buyp", "buyp:6", "buyp:6:", "buyp::7", "buyp:a:7", "buyp:6:b", "buyp:6:7:8", "buy:6:7", "buypp:6:7"} {
		t.Run(data, func(t *testing.T) {
			if _, _, err := buy.Parse(data); !errors.Is(err, callback.ErrInvalidData) {
				t.Fatalf("want ErrInvalidData, got %v", err)
			}
		})
	}
}

func TestRoute2ArgOverflow(t *testing.T) {
	r := callback.New2[int, int64]("buyp")
	if _, _, err := r.Parse("buyp:6:99999999999999999999"); !errors.Is(err, callback.ErrInvalidData) {
		t.Fatalf("err = %v", err)
	}
}

func TestRoute2MaxDataFitsTelegramLimit(t *testing.T) {
	r := callback.New2[int64, int64](strings.Repeat("a", 22))
	if n := len(r.Data(math.MinInt64, math.MinInt64)); n > 64 {
		t.Fatalf("data = %d bytes, limit 64", n)
	}
}

func TestNew2PanicsOnLongName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("panic expected")
		}
	}()
	callback.New2[int, int](strings.Repeat("a", 23))
}
