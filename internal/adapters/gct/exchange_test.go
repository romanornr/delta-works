package gct

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/romanornr/delta-works/internal/errs"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchange/accounts"
	gctexchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

type stubBotExchange struct {
	gctexchange.IBotExchange
	name               string
	tickerResp         *ticker.Price
	tickerErr          error
	holdingsResp       accounts.SubAccounts
	holdingsErr        error
	RequestedAssetType asset.Item
}

func (s *stubBotExchange) GetName() string {
	return s.name
}

func (s *stubBotExchange) UpdateTicker(_ context.Context, _ currency.Pair, _ asset.Item) (*ticker.Price, error) {
	if s.tickerErr != nil {
		return nil, s.tickerErr
	}
	return s.tickerResp, nil
}

func (s *stubBotExchange) UpdateAccountBalances(_ context.Context, a asset.Item) (accounts.SubAccounts, error) {
	s.RequestedAssetType = a
	if s.holdingsErr != nil {
		return nil, s.holdingsErr
	}
	return s.holdingsResp, nil
}

func TestExchangeAdapterFetchTickerWraps(t *testing.T) {
	adapter := NewExchange(&stubBotExchange{
		name:      "bybit",
		tickerErr: errors.New("boom"),
	}, zerolog.Nop())

	_, err := adapter.FetchTicker(context.Background(), "BTC", "USDC")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExchangeAdapterFetchHoldingsRejectsUnsupportedAccountType(t *testing.T) {
	adapter := NewExchange(&stubBotExchange{name: "bybit"}, zerolog.Nop())

	_, err := adapter.FetchHoldings(context.Background(), "margin")
	if !errors.Is(err, errs.ErrInvalidAccountType) {
		t.Errorf("expected ErrInvalidAccountType, got %v", err)
	}
}

func TestExchangeAdapterFetchHoldingsMapSpotBalances(t *testing.T) {
	balances := accounts.CurrencyBalances{}
	balances[currency.BTC] = accounts.Balance{
		Total:                  1.5,
		Free:                   1.0,
		Hold:                   0.5,
		AvailableWithoutBorrow: 1.0,
		Borrowed:               0.0,
		UpdatedAt:              time.Now(),
	}

	adapter := NewExchange(&stubBotExchange{
		name: "bybit",
		holdingsResp: accounts.SubAccounts{
			&accounts.SubAccount{ID: "main", Balances: balances},
		},
	}, zerolog.Nop())

	holdings, err := adapter.FetchHoldings(context.Background(), "spot")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	if holdings[0].Asset != "BTC" {
		t.Fatalf("expected holding asset to be BTC, got %s", holdings[0].Asset)
	}

	n1, err := decimal.NewFromString("1")
	if err != nil {
		t.Fatalf("failed to parse decimal: %v", err)
	}
	if !holdings[0].Available.Equal(n1) {
		t.Fatalf("expected holding available to be 1, got %s", holdings[0].Available)
	}

	n2, err := decimal.NewFromString("0.5")
	if err != nil {
		t.Fatalf("failed to parse decimal: %v", err)
	}
	if !holdings[0].Locked.Equal(n2) {
		t.Fatalf("expected holding locked to be 0.5, got %s", holdings[0].Locked)
	}
}
