package gct

// convert.go is the single place GCT types meet domain types. GCT carries
// prices and balances as float64; converting to decimal here is exact for
// the float64 value received but cannot restore precision the venue lost
// upstream. That is acceptable for market data and balance observations.
// Accounting truth (orders, fills, ledger) will come from venue-reported
// strings in M2, never through float64.

import (
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/ports"
)

func toGCTAsset(typ instrument.Type) (asset.Item, error) {
	switch typ {
	case instrument.TypeSpot:
		return asset.Spot, nil
	default:
		return 0, fmt.Errorf("gct: instrument type %q not supported yet", typ)
	}
}

func toGCTAssetFromAccount(acct account.Type) (asset.Item, error) {
	switch acct {
	case account.TypeSpot, account.TypeUnified:
		return asset.Spot, nil
	case account.TypeMargin:
		return asset.Margin, nil
	default:
		return 0, fmt.Errorf("%w: %q", ports.ErrUnsupportedAccount, acct)
	}
}

func toGCTPairAsset(inst instrument.Instrument) (currency.Pair, asset.Item, error) {
	item, err := toGCTAsset(inst.Type)
	if err != nil {
		return currency.EMPTYPAIR, 0, err
	}
	pair, err := currency.NewPairFromStrings(string(inst.Base), string(inst.Quote))
	if err != nil {
		return currency.EMPTYPAIR, 0, fmt.Errorf("gct: pair %s: %w", inst.Pair(), err)
	}
	return pair, item, nil
}

func toTicker(inst instrument.Instrument, p *ticker.Price) marketdata.Ticker {
	if p == nil {
		return marketdata.Ticker{Instrument: inst}
	}
	return marketdata.Ticker{
		Instrument: inst,
		Bid:        decimal.NewFromFloat(p.Bid),
		Ask:        decimal.NewFromFloat(p.Ask),
		Last:       decimal.NewFromFloat(p.Last),
		BidSize:    decimal.NewFromFloat(p.BidSize),
		AskSize:    decimal.NewFromFloat(p.AskSize),
		At:         p.LastUpdated,
	}
}

func toInstruments(venue instrument.VenueID, typ instrument.Type, pairs currency.Pairs) []instrument.Instrument {
	out := make([]instrument.Instrument, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, instrument.Instrument{
			Venue:       venue,
			Type:        typ,
			Base:        money.NewCurrency(p.Base.String()),
			Quote:       money.NewCurrency(p.Quote.String()),
			VenueSymbol: p.String(),
		})
	}
	return out
}

func toBalances(subAccounts accounts.SubAccounts) []account.Balance {
	// A venue may report several sub-accounts for one asset type; balances
	// for the same currency are summed into one observation.
	byCurrency := map[money.Currency]*account.Balance{}
	var order []money.Currency
	for _, sub := range subAccounts {
		if sub == nil {
			continue
		}
		for code, b := range sub.Balances {
			cur := money.NewCurrency(code.String())
			entry, ok := byCurrency[cur]
			if !ok {
				entry = &account.Balance{Currency: cur}
				byCurrency[cur] = entry
				order = append(order, cur)
			}
			entry.Total = entry.Total.Add(decimal.NewFromFloat(b.Total))
			entry.Free = entry.Free.Add(decimal.NewFromFloat(b.Free))
			entry.Locked = entry.Locked.Add(decimal.NewFromFloat(b.Hold))
		}
	}
	out := make([]account.Balance, 0, len(byCurrency))
	for _, cur := range order {
		out = append(out, *byCurrency[cur])
	}
	return out
}
