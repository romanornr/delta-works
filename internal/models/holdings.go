package models

import (
	"github.com/shopspring/decimal"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"time"
)

type AssetBalance struct {
	Currency               currency.Code
	Total                  decimal.Decimal
	Hold                   decimal.Decimal
	Free                   decimal.Decimal
	AvailableWithoutBorrow decimal.Decimal
	Borrowed               decimal.Decimal
	USDValue               decimal.Decimal // USDValue represents the value of an asset in USD.
}

type AccountHoldings struct {
	ExchangeName  string
	AccountType   asset.Item
	Balances      map[currency.Code]AssetBalance
	LastUpdated   time.Time
	TotalUSDValue decimal.Decimal // TotalUSDValue is a field that represents the total value in USD of an account's holdings.
}
