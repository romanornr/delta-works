package transfer

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Direction indicates whether funds are coming in or going out
type Direction string

const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// Status represents the current state of the transfer
type Status string

const (
	Pending   Status = "pending"
	Completed Status = "completed"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
)

// Type distinguishes between crypto and fiat transfers
type Type string

const (
	TypeCrypto Type = "crypto"
	TypeFiat   Type = "fiat"
)

// Transfer represents any fund movement to or from an exchange.
type Transfer struct {
	ID          string          `json:"id"`
	Exchange    string          `json:"exchange"`
	Direction   Direction       `json:"direction"`
	Type        Type            `json:"type"`
	Asset       string          `json:"asset"`
	Amount      decimal.Decimal `json:"amount"`
	Fee         decimal.Decimal `json:"fee"`
	Status      Status          `json:"status"`
	Network     string          `json:"network"`
	TxHash      string          `json:"tx_hash"`
	Address     string          `json:"address"`
	BankTo      string          `json:"bank_to"`
	Description string          `json:"description"`
	CompletedAt time.Time       `json:"completed_at"`
}

// NewTransfer creates a new transfer with required fields
func NewTransfer(exchange, id, asset string, amount decimal.Decimal, direction Direction, completedAt time.Time) *Transfer {
	return &Transfer{
		ID:          id,
		Exchange:    exchange,
		Direction:   direction,
		Asset:       asset,
		Amount:      amount,
		Fee:         decimal.Zero,
		Status:      Pending,
		CompletedAt: completedAt,
	}
}

// SetCryptoDetails sets crypto-specific transfer details
func (t *Transfer) SetCryptoDetails(network, txHash, address string) {
	t.Type = TypeCrypto
	t.Network = network
	t.TxHash = txHash
	t.Address = address
}

// SetFiatDetails sets fiat-specific transfer details
func (t *Transfer) SetFiatDetails(bankTo string) {
	t.Type = TypeFiat
	t.BankTo = bankTo
}

// SetFee sets the transfer fee
func (t *Transfer) SetFee(fee decimal.Decimal) {
	t.Fee = fee
}

// SetStatus sets the transfer status
func (t *Transfer) SetStatus(status Status) {
	t.Status = status
}

// IsCrypto reports whether the transfer is a crypto transfer
func (t *Transfer) IsCrypto() bool {
	return t.Type == TypeCrypto
}

// IsFiat reports whether the transfer is a fiat transfer
func (t *Transfer) IsFiat() bool {
	return t.Type == TypeFiat
}

// IsCompleted reports whether the transfer is completed
func (t *Transfer) IsCompleted() bool {
	return t.Status == Completed
}

// IsPending reports whether the transfer is pending
func (t *Transfer) IsPending() bool {
	return t.Status == Pending
}

// IsCancelled reports whether the transfer is cancelled
func (t *Transfer) IsCancelled() bool {
	return t.Status == Cancelled
}

// IsFailed reports whether the transfer has failed
func (t *Transfer) IsFailed() bool {
	return t.Status == Failed
}

// IsDeposit reports whether the transfer is a deposit
func (t *Transfer) IsDeposit() bool {
	return t.Direction == Inbound
}

// IsWithdrawal reports whether the transfer is a withdrawal
func (t *Transfer) IsWithdrawal() bool {
	return t.Direction == Outbound
}

// UniqueKey returns a unique identifier for duplication
func (t *Transfer) UniqueKey() string {
	return strings.ToLower(t.Exchange) + "-" + t.ID
}
