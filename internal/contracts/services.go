package contracts

import (
	"context"

	"github.com/romanornr/delta-works/internal/models"
	"github.com/thrasher-corp/gocryptotrader/currency"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/account"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

// EngineService abstracts engine operations
type EngineService interface {
	GetExchanges() []ExchangeService
	GetExchangeByName(name string) (ExchangeService, error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

// ExchangeService abstracts exchange operations
type ExchangeService interface {
	GetName() string
	UpdateAccountInfo(ctx context.Context, assetType asset.Item) (account.Holdings, error)
	UpdateTicker(ctx context.Context, pair currency.Pair, assetType asset.Item) (*ticker.Price, error)
	GetWithdrawalsHistory(ctx context.Context, currency currency.Code, assetType asset.Item) ([]exchange.WithdrawalHistory, error)
}

// RepositoryService abstracts repository operations
type RepositoryService interface {
	InsertHoldings(ctx context.Context, holdings models.AccountHoldings) error
	StoreWithdrawal(ctx context.Context, exchange string, withdrawals []exchange.WithdrawalHistory) error
	Close(ctx context.Context) error
}

type HoldingsService interface {
	UpdateHoldings(ctx context.Context, exchangeName string, accountType asset.Item) error
	GetHoldings(exchangeName string, accountType asset.Item) (*models.AccountHoldings, error)
	StartContinuousUpdate(ctx context.Context) error
	Stop(ctx context.Context) error
}

type WithdrawalService interface {
	FetchWithdrawalHistory(ctx context.Context, exchangeName string, currency currency.Code, accountType asset.Item) ([]exchange.WithdrawalHistory, error)
}
