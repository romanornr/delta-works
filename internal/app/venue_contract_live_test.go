//go:build live

package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/bus"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/log"
	"github.com/romanornr/delta-works/internal/ports/portstest"
)

func TestLiveCoinbaseCatalogContract(t *testing.T) {
	if os.Getenv("DELTA_LIVE_CONTRACT") != "GO" {
		t.Skip("DELTA_LIVE_CONTRACT=GO is required")
	}
	capValue, err := decimal.NewFromString(os.Getenv("DELTA_LIVE_NOTIONAL_CAP"))
	if err != nil || !capValue.IsPositive() {
		t.Fatal("DELTA_LIVE_NOTIONAL_CAP must be a positive decimal")
	}
	key := os.Getenv(config.EnvPrefix + "VENUES__COINBASE__API_KEY")
	secret := os.Getenv(config.EnvPrefix + "VENUES__COINBASE__API_SECRET")
	if key == "" || secret == "" {
		t.Fatal("Coinbase live credentials are required")
	}

	eventBus := bus.NewInProc()
	t.Cleanup(eventBus.Close)
	catalog, err := newVenueCatalog(config.Config{Venues: map[string]config.Venue{
		"coinbase": {
			Enabled: true, Trading: true, Accounts: []string{"spot"},
			Rate: config.Rate{RPS: 10, Burst: 10}, APIKey: key, APISecret: secret,
		},
	}}, log.Nop(), eventBus, clockwork.NewRealClock())
	if err != nil {
		t.Fatalf("build Coinbase catalog: %v", err)
	}
	entry, ok := catalog.Lookup("coinbase")
	if !ok {
		t.Fatal("Coinbase catalog entry is absent")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	accounts, ok := entry.Account()
	if !ok {
		t.Fatal("Coinbase catalog entry has no account capability")
	}
	balances, err := accounts.Balances(ctx, account.TypeSpot)
	if err != nil {
		t.Fatalf("Coinbase balances through catalog gate: %v", err)
	}
	t.Logf("fetched %d Coinbase balances", len(balances))

	placer, ok := entry.Orders()
	if !ok {
		t.Fatal("Coinbase catalog entry has no order capability")
	}
	market, ok := entry.MarketData()
	if !ok {
		t.Fatal("Coinbase catalog entry has no market-data capability")
	}
	pair := instrument.Instrument{
		Venue: "coinbase", Type: instrument.TypeSpot,
		Base: money.Currency("BTC"), Quote: money.Currency("USD"), VenueSymbol: "BTC-USD",
	}
	portstest.RunOrderPlacerContract(t, placer, portstest.Fixture{
		Instrument: pair, MinQty: decimal.RequireFromString("0.00000001"), MinNotional: capValue, MaxNotional: capValue,
		NonMarketablePrice: func(ctx context.Context) (decimal.Decimal, error) {
			ticker, err := market.Ticker(ctx, pair)
			if err != nil {
				return decimal.Zero, err
			}
			if !ticker.Bid.IsPositive() {
				return decimal.Zero, fmt.Errorf("Coinbase returned no positive bid")
			}
			return ticker.Bid.Mul(decimal.RequireFromString("0.5")), nil
		},
		EchoesClientOrderID: true, Deadline: 2 * time.Minute, Cleanup: portstest.CleanupPlacedOrders,
	})
}
