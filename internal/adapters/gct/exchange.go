package gct

import (
	"context"
	"fmt"
	"strings"

	"github.com/romanornr/delta-works/internal/domain/exchange"
	"github.com/romanornr/delta-works/internal/domain/market"
	"github.com/romanornr/delta-works/internal/domain/portfolio"
	"github.com/romanornr/delta-works/internal/errs"
	"github.com/rs/zerolog"
	"github.com/thrasher-corp/gocryptotrader/currency"
	gctexchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
)

// exchangeAdapter implements ExchangeAdapter using a single GCT exchange.
type exchangeAdapter struct {
	exch gctexchange.IBotExchange
	log  zerolog.Logger
}

// NewExchange returns an exchange.Exchange for one exchange
func NewExchange(exch gctexchange.IBotExchange, log zerolog.Logger) exchange.Exchange {
	return &exchangeAdapter{
		exch: exch,
		log:  log,
	}
}

// Name returns the normalized exchange name.
func (a *exchangeAdapter) Name() string {
	if a == nil || a.exch == nil {
		return ""
	}
	return strings.ToLower(a.exch.GetName())
}

// SupportedAccounts returns the account types supported in the first pass.
func (a *exchangeAdapter) SupportedAccounts() []string {
	return []string{"spot"}
}

// FetchTicker returns market data for trading pairs.
func (a *exchangeAdapter) FetchTicker(ctx context.Context, base, quote string) (*market.Ticker, error) {
	if a == nil || a.exch == nil {
		return nil, fmt.Errorf("failed to fetch ticker: %w", errs.ErrAdapterNotReady)
	}

	normalizedBase := currency.NewCode(base)
	normalizedQuote := currency.NewCode(quote)
	if normalizedBase.IsEmpty() || normalizedQuote.IsEmpty() {
		return nil, fmt.Errorf("failed to fetch ticker for exchange %s: base or quote is empty: %w", a.Name(), errs.ErrInvalidPair)
	}

	pair := currency.NewPair(normalizedBase, normalizedQuote)

	t, err := a.exch.UpdateTicker(ctx, pair, asset.Spot)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ticker for exchange %s pair: %s/%s: %w", a.Name(), normalizedBase, normalizedQuote, err)
	}

	result, err := ToTicker(a.exch.GetName(), t)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ticker for exchange %s pair %s/%s: %w", a.Name(), normalizedBase, normalizedQuote, err)
	}
	return result, nil
}

// FetchHoldings returns holdings for the requested account type.
func (a *exchangeAdapter) FetchHoldings(ctx context.Context, account string) ([]portfolio.Holding, error) {
	if a == nil || a.exch == nil {
		return nil, fmt.Errorf("failed to fetch holdings: %w", errs.ErrAdapterNotReady)
	}

	switch strings.ToLower(strings.TrimSpace(account)) {
	case portfolio.AccountSpot.String():
		subAccounts, err := a.exch.UpdateAccountBalances(ctx, asset.Spot)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch spot holdings for exchange %s: %w", a.Name(), err)
		}

		holdings, err := ToHoldings(subAccounts)
		if err != nil {
			return nil, fmt.Errorf("failed to convert holdings for exchange %s account %s: %w", a.Name(), account, err)
		}
		return holdings, nil
	default:
		return nil, fmt.Errorf("failed to fetch holdings for exchange %s account %s: %w", a.Name(), account, errs.ErrInvalidAccountType)
	}
}
