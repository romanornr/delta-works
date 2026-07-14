//go:build live

package gct

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
	"github.com/romanornr/delta-works/internal/ports/portstest"
)

func TestLiveCoinbaseOrderPlacerContract(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	exchange, err := New(ctx, "coinbase", config.Venue{APIKey: key, APISecret: secret})
	if err != nil {
		t.Fatalf("setup Coinbase: %v", err)
	}
	pair := instrument.Instrument{Venue: "coinbase", Type: instrument.TypeSpot, Base: money.Currency("BTC"), Quote: money.Currency("USD"), VenueSymbol: "BTC-USD"}
	portstest.RunOrderPlacerContract(t, exchange, portstest.Fixture{
		Instrument: pair, MinQty: decimal.RequireFromString("0.00000001"), MinNotional: capValue,
		NonMarketablePrice: func(ctx context.Context) (decimal.Decimal, error) {
			ticker, err := exchange.Ticker(ctx, pair)
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
