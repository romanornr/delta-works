package gct

import (
	"testing"
)

func TestSupportFor(t *testing.T) {
	t.Parallel()

	accountAndMarket := Support{Account: true, MarketData: true}
	want := map[string]Support{
		"binance":     accountAndMarket,
		"binanceus":   accountAndMarket,
		"bitfinex":    accountAndMarket,
		"bitflyer":    {MarketData: true},
		"bithumb":     accountAndMarket,
		"bitmex":      accountAndMarket,
		"bitstamp":    accountAndMarket,
		"btc markets": accountAndMarket,
		"btse":        accountAndMarket,
		"bybit":       accountAndMarket,
		"coinbase":    {Account: true, MarketData: true, Orders: true, PrivateEvents: true},
		"coinut":      accountAndMarket,
		"deribit":     accountAndMarket,
		"exmo":        accountAndMarket,
		"gateio":      accountAndMarket,
		"gemini":      accountAndMarket,
		"hitbtc":      accountAndMarket,
		"huobi":       accountAndMarket,
		"kraken":      accountAndMarket,
		"kucoin":      accountAndMarket,
		"lbank":       accountAndMarket,
		"okx":         accountAndMarket,
		"poloniex":    accountAndMarket,
		"yobit":       accountAndMarket,
	}
	if len(supportByVenue) != len(want) {
		t.Fatalf("support entries = %d, want %d", len(supportByVenue), len(want))
	}
	for name, expected := range want {
		got, ok := SupportFor(name)
		if !ok || got != expected {
			t.Errorf("SupportFor(%q) = (%+v, %t), want (%+v, true)", name, got, ok, expected)
		}
	}
	for _, name := range []string{"unknown", "Coinbase", " coinbase"} {
		if got, ok := SupportFor(name); ok || got != (Support{}) {
			t.Errorf("SupportFor(%q) = (%+v, %t), want zero, false", name, got, ok)
		}
	}
}
