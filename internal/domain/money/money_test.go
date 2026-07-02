package money

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestNewCurrencyCanonicalizes(t *testing.T) {
	if got := NewCurrency(" btc "); got != "BTC" {
		t.Errorf("got %q, want BTC", got)
	}
}

func TestAddSub(t *testing.T) {
	a := NewAmount("BTC", dec("1.5"))
	b := NewAmount("BTC", dec("0.25"))

	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Decimal().Equal(dec("1.75")) {
		t.Errorf("Add: got %s", sum)
	}

	diff, err := a.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Decimal().Equal(dec("1.25")) {
		t.Errorf("Sub: got %s", diff)
	}
}

func TestCurrencyMismatch(t *testing.T) {
	btc := NewAmount("BTC", dec("1"))
	eth := NewAmount("ETH", dec("1"))

	if _, err := btc.Add(eth); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add mismatch: got %v", err)
	}
	if _, err := btc.Sub(eth); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sub mismatch: got %v", err)
	}
	// The zero Amount is not silently compatible with anything.
	if _, err := btc.Add(Amount{}); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add zero Amount: got %v", err)
	}
}

func TestMulAndPredicates(t *testing.T) {
	a := NewAmount("USDT", dec("10")).Mul(dec("-0.5"))
	if a.Currency() != "USDT" || !a.Decimal().Equal(dec("-5")) {
		t.Errorf("Mul: got %s", a)
	}
	if !a.IsNegative() {
		t.Error("IsNegative: expected true")
	}
	if !NewAmount("USDT", decimal.Zero).IsZero() {
		t.Error("IsZero: expected true")
	}
	if got := a.String(); got != "-5 USDT" {
		t.Errorf("String: got %q", got)
	}
}

// Exactness guard: repeated decimal addition must not drift the way binary
// floats do (0.1+0.2 != 0.3 in float64).
func TestNoFloatDrift(t *testing.T) {
	sum := NewAmount("USDT", decimal.Zero)
	var err error
	for range 10 {
		sum, err = sum.Add(NewAmount("USDT", dec("0.1")))
		if err != nil {
			t.Fatal(err)
		}
	}
	if !sum.Decimal().Equal(dec("1")) {
		t.Errorf("drift: got %s, want exactly 1", sum)
	}
}
