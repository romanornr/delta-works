// Package ledger models per-bot inventory lots and pure lot selection.
package ledger

import (
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/instrument"
	"github.com/romanornr/delta-works/internal/domain/money"
)

// Lot is an open inventory position created by one buy fill.
type Lot struct {
	ID           string // ULID, opaque to the domain
	BotID        string
	Venue        instrument.VenueID
	Base, Quote  money.Currency
	Qty          decimal.Decimal
	RemainingQty decimal.Decimal
	CostPrice    decimal.Decimal // opening fill's execution price; zero when the venue reported none
	OpenedAt     time.Time
}

// Closure is one lot's share of a sell fill.
type Closure struct {
	LotID string
	Qty   decimal.Decimal
}

// Allocation is a selector's answer for one sell fill.
type Allocation struct {
	Closures  []Closure
	Unmatched decimal.Decimal
}

// Outcome reports the observable result of applying a fill to the ledger.
type Outcome struct {
	UnmatchedQty decimal.Decimal
}

// LotSelector decides which open lots a sell fill closes. Pure: same
// inputs, same answer, input slice never mutated. sellQty must be
// positive; the caller validates before selecting.
type LotSelector interface {
	Select(open []Lot, sellQty decimal.Decimal) Allocation
}

// FIFO closes oldest lots first, ties broken by lot ID.
type FIFO struct{}

// Select allocates sellQty across open lots in FIFO order.
func (FIFO) Select(open []Lot, sellQty decimal.Decimal) Allocation {
	lots := append([]Lot(nil), open...)
	sort.Slice(lots, func(i, j int) bool {
		if lots[i].OpenedAt.Equal(lots[j].OpenedAt) {
			return lots[i].ID < lots[j].ID
		}
		return lots[i].OpenedAt.Before(lots[j].OpenedAt)
	})

	remaining := sellQty
	allocation := Allocation{}
	for _, lot := range lots {
		if !remaining.IsPositive() {
			break
		}
		if !lot.RemainingQty.IsPositive() {
			continue
		}
		qty := decimal.Min(remaining, lot.RemainingQty)
		allocation.Closures = append(allocation.Closures, Closure{LotID: lot.ID, Qty: qty})
		remaining = remaining.Sub(qty)
	}
	allocation.Unmatched = remaining
	return allocation
}
