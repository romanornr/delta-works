package gct

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
)

func btcusdt() instrument.Instrument {
	return instrument.Instrument{
		Venue: "bybit", Type: instrument.TypeSpot,
		Base: "BTC", Quote: "USDT", VenueSymbol: "BTCUSDT",
	}
}

func TestToGCTPairAsset(t *testing.T) {
	pair, item, err := toGCTPairAsset(btcusdt())
	if err != nil {
		t.Fatal(err)
	}
	if item != asset.Spot {
		t.Errorf("asset: got %v", item)
	}
	if pair.Base.String() != "BTC" || pair.Quote.String() != "USDT" {
		t.Errorf("pair: got %s", pair)
	}
}

func TestToGCTPairAssetUnsupportedType(t *testing.T) {
	inst := btcusdt()
	inst.Type = "futures"
	if _, _, err := toGCTPairAsset(inst); err == nil {
		t.Error("expected error for unsupported instrument type")
	}
}

func TestToTicker(t *testing.T) {
	at := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	got := toTicker(btcusdt(), &ticker.Price{
		Bid: 50000.5, Ask: 50001, Last: 50000.75, BidSize: 1.5, AskSize: 2, LastUpdated: at,
	})
	if !got.Bid.Equal(decimal.NewFromFloat(50000.5)) || !got.Ask.Equal(decimal.NewFromInt(50001)) {
		t.Errorf("bid/ask: got %s/%s", got.Bid, got.Ask)
	}
	if !got.At.Equal(at) {
		t.Errorf("At: got %s", got.At)
	}
	if got.Instrument.Key() != "bybit:spot:BTC/USDT" {
		t.Errorf("instrument: got %s", got.Instrument.Key())
	}
}

func TestToBalancesSumsAcrossSubAccounts(t *testing.T) {
	subs := accounts.SubAccounts{
		{
			ID: "main", AssetType: asset.Spot,
			Balances: accounts.CurrencyBalances{
				currency.BTC:  {Currency: currency.BTC, Total: 1.5, Free: 1, Hold: 0.5},
				currency.USDT: {Currency: currency.USDT, Total: 100, Free: 100},
			},
		},
		{
			ID: "sub1", AssetType: asset.Spot,
			Balances: accounts.CurrencyBalances{
				currency.BTC: {Currency: currency.BTC, Total: 0.5, Free: 0.5},
			},
		},
		nil, // defensive: adapters must tolerate nil entries
	}

	got := toBalances(subs)
	byCur := map[string]account.Balance{}
	for _, b := range got {
		byCur[string(b.Currency)] = b
	}

	btc := byCur["BTC"]
	if !btc.Total.Equal(decimal.NewFromInt(2)) || !btc.Free.Equal(decimal.NewFromFloat(1.5)) || !btc.Locked.Equal(decimal.NewFromFloat(0.5)) {
		t.Errorf("BTC: got total=%s free=%s locked=%s", btc.Total, btc.Free, btc.Locked)
	}
	if usdt := byCur["USDT"]; !usdt.Total.Equal(decimal.NewFromInt(100)) {
		t.Errorf("USDT: got total=%s", usdt.Total)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 currencies, got %d", len(got))
	}
}

func TestToInstruments(t *testing.T) {
	pairs := currency.Pairs{currency.NewPair(currency.BTC, currency.USDT)}
	got := toInstruments("bybit", instrument.TypeSpot, pairs)
	if len(got) != 1 {
		t.Fatalf("expected 1 instrument, got %d", len(got))
	}
	if got[0].Key() != "bybit:spot:BTC/USDT" {
		t.Errorf("key: got %s", got[0].Key())
	}
	if got[0].VenueSymbol == "" {
		t.Error("VenueSymbol must be populated")
	}
}
