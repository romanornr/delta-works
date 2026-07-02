// Package account models venue accounts and their balances.
package account

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
)

// Type is a venue account classification.
type Type string

// Account classifications. Venues differ; adapters map native names onto these.
const (
	TypeSpot    Type = "spot"
	TypeFunding Type = "funding"
	TypeUnified Type = "unified"
	TypeMargin  Type = "margin"
)

// Ref identifies one account at one venue.
type Ref struct {
	Venue instrument.VenueID
	Type  Type
}

// Balance is the holding of one currency within an account.
type Balance struct {
	Currency money.Currency
	Total    decimal.Decimal
	Free     decimal.Decimal
	Locked   decimal.Decimal
}

// Snapshot is a point-in-time view of an account's balances.
type Snapshot struct {
	Account  Ref
	TakenAt  time.Time
	Balances []Balance
}

// NonZero returns only balances with a non-zero total.
func (s Snapshot) NonZero() []Balance {
	var out []Balance
	for _, b := range s.Balances {
		if !b.Total.IsZero() {
			out = append(out, b)
		}
	}
	return out
}
