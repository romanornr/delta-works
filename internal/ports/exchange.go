package ports

import (
	"context"
	"errors"

	"github.com/romanornr/delta-works/internal/domain/account"
	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/marketdata"
)

// Typed errors adapters return so resilience layers and services can
// classify failures without knowing the underlying library.
var (
	ErrVenueUnavailable   = errors.New("venue unavailable")
	ErrAuth               = errors.New("authentication failed")
	ErrRateLimited        = errors.New("rate limited by venue")
	ErrUnsupportedAccount = errors.New("unsupported account type")
	ErrTradingUnsupported = errors.New("venue adapter does not support trading")
)

// MarketDataReader provides public market data.
type MarketDataReader interface {
	Ticker(ctx context.Context, inst instrument.Instrument) (marketdata.Ticker, error)
	Instruments(ctx context.Context, typ instrument.Type) ([]instrument.Instrument, error)
}

// AccountReader provides private account data.
type AccountReader interface {
	Balances(ctx context.Context, acct account.Type) ([]account.Balance, error)
}

// Exchange is one venue connection. Trading capability lives in a separate
// port (see trading.go) so read-only consumers can never place orders by
// accident.
type Exchange interface {
	ID() instrument.VenueID
	MarketDataReader
	AccountReader
}
