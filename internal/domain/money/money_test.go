package money

import "testing"

func TestNewCurrencyCanonicalizes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Currency
	}{
		{name: "trims and uppercases", input: " btc ", want: "BTC"},
		{name: "preserves canonical code", input: "USDT", want: "USDT"},
		{name: "canonicalizes mixed case", input: "Eth", want: "ETH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCurrency(tt.input); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
