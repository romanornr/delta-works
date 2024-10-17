package models

import "time"

// Withdrawal represents a record of funds being taken out from an account.
type Withdrawal struct {
	Exchange        string
	Status          string
	TransferID      string
	Description     string
	Currency        string
	Amount          float64
	Fee             float64
	TransferType    string
	CryptoToAddress string
	CryptoTxID      string
	CryptoChain     string
	BankTo          string
	Timestamp       time.Time
}
