package portfolio

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestSnapshotAddHoldingReplacesExistingAssetValue(t *testing.T) {
	snapshot := NewSnapshot("test-exchange", AccountSpot, time.Unix(0, 0))

	snapshot.AddHolding(NewHolding(
		"ETH",
		decimal.NewFromInt(1),
		decimal.NewFromInt(1),
		decimal.Zero,
		decimal.NewFromInt(1),
		decimal.Zero,
		decimal.NewFromInt(75),
	))
	snapshot.AddHolding(NewHolding(
		"BTC",
		decimal.NewFromInt(1),
		decimal.NewFromInt(1),
		decimal.Zero,
		decimal.NewFromInt(1),
		decimal.Zero,
		decimal.NewFromInt(100),
	))
	replacement := NewHolding(
		"BTC",
		decimal.NewFromInt(2),
		decimal.NewFromInt(2),
		decimal.Zero,
		decimal.NewFromInt(2),
		decimal.Zero,
		decimal.NewFromInt(250),
	)

	snapshot.AddHolding(replacement)

	holding, ok := snapshot.Holdings["BTC"]
	if !ok {
		t.Fatal("expected BTC holding to exist")
	}
	if holding.Asset != replacement.Asset || !holding.Total.Equal(replacement.Total) || !holding.Value.Equal(replacement.Value) {
		t.Fatalf("expected BTC replacement total/value %s/%s, got %s/%s", replacement.Total, replacement.Value, holding.Total, holding.Value)
	}

	expectedTotal := decimal.NewFromInt(325)
	if !snapshot.TotalValue.Equal(expectedTotal) {
		t.Fatalf("expected total value %s, got %s", expectedTotal, snapshot.TotalValue)
	}
}
