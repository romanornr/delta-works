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
}

type AccountHoldings struct {
	ExchangeName string
	AccountType  asset.Item
	Balances     map[currency.Code]AssetBalance
	LastUpdated  time.Time
}
