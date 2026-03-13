package gct

import (
	"context"

	"github.com/romanornr/delta-works/internal/domain/market"
	"github.com/romanornr/delta-works/internal/domain/portfolio"
)

// ExchangeAdapter defines the exchange-facing contract currently used by Delta Works.
type ExchangeAdapter interface {
	FetchTicker(ctx context.Context, base, quote string) (*market.Ticker, error)
	FetchHoldings(ctx context.Context, account string) ([]portfolio.Holding, error)
	Name() string
	SupportedAccounts() []string
}

// Registry provides normalized lookup for exchange adapters.
type Registry interface {
	Get(exchangeName string) (ExchangeAdapter, error)
	All() []ExchangeAdapter
	Names() []string
}
