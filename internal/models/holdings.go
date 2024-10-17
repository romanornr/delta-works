package models

import (
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"time"
)

// AssetBalance represents the balance of a specific asset in an account.
type AssetBalance struct {
	Currency               currency.Code
	Total                  decimal.Decimal
	Hold                   decimal.Decimal
	Free                   decimal.Decimal
	AvailableWithoutBorrow decimal.Decimal
	Borrowed               decimal.Decimal
	USDValue               decimal.Decimal // USDValue represents the value of an asset in USD.
}

// AccountHoldings represents the account holdings for a specific exchange and account type.
// It includes the exchange name, account type, asset balances, last update time, and total value in USD.
type AccountHoldings struct {
	ExchangeName  string
	AccountType   asset.Item
	Balances      map[currency.Code]AssetBalance
	LastUpdated   time.Time
	TotalUSDValue decimal.Decimal // TotalUSDValue is a field that represents the total value in USD of an account's holdings.
}
