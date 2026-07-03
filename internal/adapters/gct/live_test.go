//go:build live

package gct

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/config"
	"github.com/romanornr/delta-works/internal/domain/account"
)

// TestLiveBalances verifies real venue credentials end to end. Manual only:
//
//	DELTA__VENUES__BYBIT__API_KEY=... DELTA__VENUES__BYBIT__API_SECRET=... \
//	go test -tags live -run TestLiveBalances -v ./internal/adapters/gct
func TestLiveBalances(t *testing.T) {
	key := os.Getenv("DELTA__VENUES__BYBIT__API_KEY")
	secret := os.Getenv("DELTA__VENUES__BYBIT__API_SECRET")
	if key == "" || secret == "" {
		t.Skip("venue credentials not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ex, err := New(ctx, "bybit", config.Venue{APIKey: key, APISecret: secret})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	balances, err := ex.Balances(ctx, account.TypeSpot)
	if err != nil {
		t.Fatalf("balances: %v", err)
	}
	t.Logf("fetched %d balances", len(balances))
	for _, b := range balances {
		t.Logf("%s total=%s free=%s locked=%s", b.Currency, b.Total, b.Free, b.Locked)
	}
}
