package models

import (
	"github.com/shopspring/decimal"
	"time"
)

type Deposit struct {
	Exchange          string
	Status            string
	TransferID        string
	Description       string
	currency          string
	Amount            decimal.Decimal
	TransferType      string
	CryptoFromAddress string
	CryptoToAddress   string
	CryptoTxID        string
	BankFrom          string
	Timestamp         time.Time
}

type DepositAddress struct {
	Currency  string
	Address   string
	Chain     string
	Tag       string // Tag is an optional for currencies that require a tag/memo
	Timestamp time.Time
}
