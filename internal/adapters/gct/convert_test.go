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

type fakeMatcher struct {
	pair       currency.Pair
	err        error
	gotSymbol  string
	gotHasDlim bool
}

func (f *fakeMatcher) MatchSymbolWithAvailablePairs(symbol string, _ asset.Item, hasDelimiter bool) (currency.Pair, error) {
	f.gotSymbol, f.gotHasDlim = symbol, hasDelimiter
	return f.pair, f.err
}

func TestToGCTPairAsset(t *testing.T) {
	dogeusdt := currency.NewPair(currency.DOGE, currency.USDT)

	tests := []struct {
		name       string
		inst       instrument.Instrument
		matcher    *fakeMatcher
		wantPair   string
		wantDelim  bool
		wantSymbol string
	}{
		{
			name: "venue symbol matched against venue pairs",
			inst: instrument.Instrument{
				Venue: "bybit", Type: instrument.TypeSpot,
				Base: "DOGE", Quote: "USDT", VenueSymbol: "DOGEUSDT",
			},
			matcher:    &fakeMatcher{pair: dogeusdt},
			wantPair:   "DOGEUSDT",
			wantSymbol: "DOGEUSDT",
		},
		{
			name: "delimited venue symbol flagged for the matcher",
			inst: instrument.Instrument{
				Venue: "kraken", Type: instrument.TypeSpot,
				Base: "DOGE", Quote: "USDT", VenueSymbol: "DOGE-USDT",
			},
			matcher:    &fakeMatcher{pair: dogeusdt},
			wantPair:   "DOGEUSDT",
			wantDelim:  true,
			wantSymbol: "DOGE-USDT",
		},
		{
			name: "no venue symbol falls back to base and quote",
			inst: instrument.Instrument{
				Venue: "bybit", Type: instrument.TypeSpot,
				Base: "BTC", Quote: "USDT",
			},
			matcher:  &fakeMatcher{err: currency.ErrPairNotFound},
			wantPair: "BTCUSDT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair, item, err := toGCTPairAsset(tt.matcher, tt.inst)
			if err != nil {
				t.Fatal(err)
			}
			if item != asset.Spot {
				t.Errorf("asset: got %v", item)
			}
			if got := pair.Base.String() + pair.Quote.String(); got != tt.wantPair {
				t.Errorf("pair: got %s, want %s", got, tt.wantPair)
			}
			if tt.matcher.gotSymbol != tt.wantSymbol {
				t.Errorf("matched symbol: got %q, want %q", tt.matcher.gotSymbol, tt.wantSymbol)
			}
			if tt.matcher.gotHasDlim != tt.wantDelim {
				t.Errorf("hasDelimiter: got %v, want %v", tt.matcher.gotHasDlim, tt.wantDelim)
			}
		})
	}
}

func TestToGCTPairAssetUnmatchedSymbolFails(t *testing.T) {
	inst := btcusdt()
	if _, _, err := toGCTPairAsset(&fakeMatcher{err: currency.ErrPairNotFound}, inst); err == nil {
		t.Error("expected error for a venue symbol the venue does not list")
	}
}

func TestToGCTPairAssetUnsupportedType(t *testing.T) {
	inst := btcusdt()
	inst.Type = "futures"
	if _, _, err := toGCTPairAsset(&fakeMatcher{}, inst); err == nil {
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
