package instrument

import (
	"testing"
)

func TestParsePair(t *testing.T) {
	tests := []struct {
		in        string
		base      string
		quote     string
		wantError bool
	}{
		{in: "BTC/USDT", base: "BTC", quote: "USDT"},
		{in: "btc/usdt", base: "BTC", quote: "USDT"},
		{in: " eth /usd ", base: "ETH", quote: "USD"},
		{in: "BTCUSDT", wantError: true},
		{in: "", wantError: true},
		{in: "/USDT", wantError: true},
		{in: "BTC/", wantError: true},
		{in: "BTC/USDT/PERP", wantError: true},
		{in: "/", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			base, quote, err := ParsePair(tt.in)
			if tt.wantError {
				if err == nil {
					t.Errorf("ParsePair(%q): expected error, got %s/%s", tt.in, base, quote)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePair(%q): %v", tt.in, err)
			}
			if string(base) != tt.base || string(quote) != tt.quote {
				t.Errorf("ParsePair(%q) = %s/%s, want %s/%s", tt.in, base, quote, tt.base, tt.quote)
			}
		})
	}
}

func TestKeyStability(t *testing.T) {
	i := Instrument{
		Venue: NewVenueID("Bybit"), Type: TypeSpot,
		Base: "BTC", Quote: "USDT", VenueSymbol: "BTCUSDT",
	}
	// Key format is a stable contract: it is used as a map key and may end
	// up persisted. Changing it is a breaking change.
	if got := i.Key(); got != "bybit:spot:BTC/USDT" {
		t.Errorf("Key: got %q", got)
	}
	if got := i.Pair(); got != "BTC/USDT" {
		t.Errorf("Pair: got %q", got)
	}
}
