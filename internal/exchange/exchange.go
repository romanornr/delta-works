package exchange

import (
	"context"

	"github.com/romanornr/delta-works/internal/domain/market"
	"github.com/romanornr/delta-works/internal/domain/portfolio"
)

// Exchange provides market and balance data for a single exchange
// It is an application-facing port for fetching tickers and holdings
// without depending on a specific exchange backend
type Exchange interface {
	FetchTicker(ctx context.Context, base, quote string) (*market.Ticker, error)
	FetchHoldings(ctx context.Context, account string) ([]portfolio.Holding, error)
	Name() string
	SupportedAccounts() []string
}

// Registry provides normalized lookup for exchange implementations.
type Registry interface {
	Get(exchangeName string) (Exchange, error)
	All() []Exchange
	Names() []string
}
