// Package gct adapts gocryptotrader to the application's ports. This is the
// ONLY package allowed to import gocryptotrader (depguard-enforced,
// ADR-0003). Each venue is a standalone GCT exchange configured from our
// config. The full GCT engine (config file, database, comms, web) is never
// booted.
package gct

import (
	"context"
	"fmt"

	gctconfig "github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/engine"
	gctexchange "github.com/thrasher-corp/gocryptotrader/exchanges"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
	"github.com/romanornr/delta-works/internal/ports"
)

// Exchange implements ports.Exchange over one GCT exchange.
type Exchange struct {
	id   instrument.VenueID
	exch gctexchange.IBotExchange
}

var _ ports.Exchange = (*Exchange)(nil)

// New instantiates and configures a GCT exchange for the named venue.
// Credentials are optional; without them only public endpoints work.
func New(ctx context.Context, venue string, cfg config.Venue) (*Exchange, error) {
	exch, err := engine.NewSupportedExchangeByName(venue)
	if err != nil {
		return nil, fmt.Errorf("gct: unsupported venue %q: %w", venue, err)
	}

	defaultCfg, err := gctexchange.GetDefaultConfig(ctx, exch)
	if err != nil {
		return nil, fmt.Errorf("gct: default config for %q: %w", venue, err)
	}
	applyCredentials(defaultCfg, cfg)

	if err := exch.Setup(defaultCfg); err != nil {
		return nil, fmt.Errorf("gct: setup %q: %w", venue, err)
	}

	return &Exchange{id: instrument.NewVenueID(venue), exch: exch}, nil
}

func applyCredentials(defaultCfg *gctconfig.Exchange, cfg config.Venue) {
	if cfg.APIKey == "" {
		return
	}
	defaultCfg.API.AuthenticatedSupport = true
	defaultCfg.API.Credentials.Key = cfg.APIKey
	defaultCfg.API.Credentials.Secret = cfg.APISecret
}

// ID implements ports.Exchange.
func (e *Exchange) ID() instrument.VenueID { return e.id }

// Ticker implements ports.MarketDataReader.
func (e *Exchange) Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error) {
	pair, item, err := toGCTPairAsset(inst)
	if err != nil {
		return marketdata.Ticker{}, err
	}
	price, err := e.exch.UpdateTicker(ctx, pair, item)
	if err != nil {
		return marketdata.Ticker{}, fmt.Errorf("gct: ticker %s %s: %w", e.id, inst.Pair(), err)
	}
	return toTicker(inst, price), nil
}

// Instruments implements ports.MarketDataReader. It returns the venue's
// enabled instruments for the given type.
func (e *Exchange) Instruments(_ context.Context, typ instrument.Type) ([]instrument.Instrument, error) {
	item, err := toGCTAsset(typ)
	if err != nil {
		return nil, err
	}
	pairs, err := e.exch.GetAvailablePairs(item)
	if err != nil {
		return nil, fmt.Errorf("gct: available pairs %s: %w", e.id, err)
	}
	return toInstruments(e.id, typ, pairs), nil
}

// Balances implements ports.AccountReader.
func (e *Exchange) Balances(ctx context.Context, acct account.Type) ([]account.Balance, error) {
	item, err := toGCTAssetFromAccount(acct)
	if err != nil {
		return nil, err
	}
	subAccounts, err := e.exch.UpdateAccountBalances(ctx, item)
	if err != nil {
		return nil, fmt.Errorf("gct: balances %s %s: %w", e.id, acct, err)
	}
	return toBalances(subAccounts), nil
}
