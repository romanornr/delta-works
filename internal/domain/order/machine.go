package order

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// The state machine is specified in docs/specs/m2-oms.md. Statuses have a
// rank used as a monotonic guard against stale or reordered venue events;
// all terminal statuses share the top rank because none can follow another.
func rank(s Status) (int, error) {
	switch s {
	case StatusPending:
		return 0, nil
	case StatusOpen:
		return 1, nil
	case StatusPartiallyFilled:
		return 2, nil
	case StatusFilled, StatusCanceled, StatusRejected, StatusExpired:
		return 3, nil
	default:
		return 0, fmt.Errorf("order: unknown status %q", s)
	}
}

const terminalRank = 3

// Terminal reports whether s admits no further status changes.
func (s Status) Terminal() bool {
	r, err := rank(s)
	return err == nil && r == terminalRank
}

// State is the stored view of an order that Transition decides against.
type State struct {
	Status    Status
	FilledQty decimal.Decimal
}

// DropReason classifies why an event's status change was not applied.
type DropReason string

// Drop reasons. Events dropped with a zero FillDelta contributed nothing
// and are counted in order_events_dropped_total by the caller.
const (
	DropStale     DropReason = "stale"
	DropDuplicate DropReason = "duplicate"
	DropTerminal  DropReason = "terminal"
)

// Decision is what applying an event to a stored order should do. A
// decision can carry a fill without a status transition: execution facts
// are extracted even when the status itself is stale. A rank-advancing
// status can also apply while its regressed fill claim is rejected.
type Decision struct {
	Transition  bool            // record a transition row and set the status to To
	To          Status          // resulting status; equals the stored status unless Transition
	FillDelta   decimal.Decimal // positive: record a fill of this quantity
	FillAnomaly bool            // a rank-advancing event carried a regressed cumulative fill; the status applied, the fill claim was rejected
	Drop        DropReason      // set when the status change was not applied
}

// Transition decides how a venue event applies to the stored state. It is
// pure and idempotent: replaying an already applied event yields a drop
// with a zero FillDelta. Event.FilledQty is cumulative; the fill delta is
// computed against the stored value, which makes apply order-independent
// across the ack, stream, and reconcile sources.
func Transition(current State, ev Event) (Decision, error) {
	curRank, err := rank(current.Status)
	if err != nil {
		return Decision{}, err
	}
	evRank, err := rank(ev.Status)
	if err != nil {
		return Decision{}, err
	}

	delta := ev.FilledQty.Sub(current.FilledQty)

	// A pending event carries no execution facts: fills cannot exist
	// before the venue has accepted the order.
	fill := decimal.Zero
	if delta.IsPositive() && ev.Status != StatusPending {
		fill = delta
	}

	switch {
	case curRank == terminalRank:
		return Decision{To: current.Status, FillDelta: fill, Drop: DropTerminal}, nil
	case delta.IsNegative() && evRank <= curRank:
		// A fill regression from a same- or lower-rank event is ordinary
		// out-of-order traffic. Never un-fill.
		return Decision{To: current.Status, Drop: DropStale}, nil
	case delta.IsNegative():
		// Venue state still advances while its regressed fill claim is
		// rejected and counted by the caller.
		return Decision{Transition: true, To: ev.Status, FillAnomaly: true}, nil
	case evRank < curRank:
		// Rank-regressing events are ordinary out-of-order traffic; a
		// lower cumulative there is expected, not an anomaly.
		return Decision{To: current.Status, FillDelta: fill, Drop: DropStale}, nil
	case evRank > curRank:
		return Decision{Transition: true, To: ev.Status, FillDelta: fill}, nil
	default:
		if ev.Status == StatusPartiallyFilled && fill.IsPositive() {
			// Another partial fill: same status, new cumulative quantity.
			return Decision{Transition: true, To: ev.Status, FillDelta: fill}, nil
		}
		return Decision{To: current.Status, FillDelta: fill, Drop: DropDuplicate}, nil
	}
}
