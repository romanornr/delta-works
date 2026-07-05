package order_test

import (
	"fmt"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/order"
)

var allStatuses = []order.Status{
	order.StatusPending,
	order.StatusOpen,
	order.StatusPartiallyFilled,
	order.StatusFilled,
	order.StatusCanceled,
	order.StatusRejected,
	order.StatusExpired,
}

// applies encodes the spec's transition table (docs/specs/m2-oms.md) as
// literal data: true means the event's status is applied when it brings a
// new cumulative fill or the drop rule does not fire. It is written out
// per pair, not derived, so the test is independent of the implementation.
var applies = map[order.Status]map[order.Status]bool{
	order.StatusPending: {
		order.StatusOpen: true, order.StatusPartiallyFilled: true,
		order.StatusFilled: true, order.StatusCanceled: true,
		order.StatusRejected: true, order.StatusExpired: true,
	},
	order.StatusOpen: {
		order.StatusPartiallyFilled: true,
		order.StatusFilled:          true, order.StatusCanceled: true,
		order.StatusRejected: true, order.StatusExpired: true,
	},
	order.StatusPartiallyFilled: {
		order.StatusPartiallyFilled: true, // only with a new cumulative fill
		order.StatusFilled:          true, order.StatusCanceled: true,
		order.StatusRejected: true, order.StatusExpired: true,
	},
	order.StatusFilled:   {},
	order.StatusCanceled: {},
	order.StatusRejected: {},
	order.StatusExpired:  {},
}

var testRank = map[order.Status]int{
	order.StatusPending:         0,
	order.StatusOpen:            1,
	order.StatusPartiallyFilled: 2,
	order.StatusFilled:          3,
	order.StatusCanceled:        3,
	order.StatusRejected:        3,
	order.StatusExpired:         3,
}

func wantDecision(stored, ev order.Status, delta int) order.Decision {
	if delta < 0 {
		// Only a same-or-higher-rank event shrinking the cumulative fill
		// is an anomaly; stale and post-terminal events with lower
		// cumulative are ordinary out-of-order traffic.
		switch {
		case testRank[stored] == 3:
			return order.Decision{To: stored, Drop: order.DropTerminal}
		case testRank[ev] < testRank[stored]:
			return order.Decision{To: stored, Drop: order.DropStale}
		default:
			return order.Decision{To: stored, Drop: order.DropNegativeFill}
		}
	}

	// Execution facts are extracted from any non-pending event, even when
	// its status is dropped. Pending events carry no fills.
	fill := decimal.Zero
	if delta > 0 && ev != order.StatusPending {
		fill = decimal.NewFromInt(int64(delta))
	}

	apply := applies[stored][ev]
	if stored == order.StatusPartiallyFilled && ev == order.StatusPartiallyFilled && delta == 0 {
		apply = false // same-rank repeat with no new cumulative fill
	}
	if apply {
		return order.Decision{Transition: true, To: ev, FillDelta: fill}
	}

	var drop order.DropReason
	switch {
	case stored.Terminal():
		drop = order.DropTerminal
	case stored == ev:
		drop = order.DropDuplicate
	default:
		drop = order.DropStale
	}
	return order.Decision{To: stored, FillDelta: fill, Drop: drop}
}

func TestTransitionExhaustive(t *testing.T) {
	t.Parallel()

	const storedFilled = 5
	for _, stored := range allStatuses {
		for _, ev := range allStatuses {
			for _, delta := range []int{-1, 0, 1} {
				name := fmt.Sprintf("%s/%s/delta=%d", stored, ev, delta)
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					current := order.State{
						Status:    stored,
						FilledQty: decimal.NewFromInt(storedFilled),
					}
					event := order.Event{
						Status:    ev,
						FilledQty: decimal.NewFromInt(storedFilled + int64(delta)),
					}
					want := wantDecision(stored, ev, delta)

					got, err := order.Transition(current, event)
					if err != nil {
						t.Fatalf("Transition: %v", err)
					}
					if got.Transition != want.Transition || got.To != want.To || got.Drop != want.Drop {
						t.Fatalf("Transition = %+v, want %+v", got, want)
					}
					if !got.FillDelta.Equal(want.FillDelta) {
						t.Fatalf("FillDelta = %s, want %s", got.FillDelta, want.FillDelta)
					}
				})
			}
		}
	}
}

func TestTransitionUnknownStatus(t *testing.T) {
	t.Parallel()

	if _, err := order.Transition(order.State{Status: "bogus"}, order.Event{Status: order.StatusOpen}); err == nil {
		t.Fatal("unknown stored status: want error")
	}
	if _, err := order.Transition(order.State{Status: order.StatusOpen}, order.Event{Status: "bogus"}); err == nil {
		t.Fatal("unknown event status: want error")
	}
}

func TestTerminal(t *testing.T) {
	t.Parallel()

	terminal := map[order.Status]bool{
		order.StatusFilled: true, order.StatusCanceled: true,
		order.StatusRejected: true, order.StatusExpired: true,
	}
	for _, s := range allStatuses {
		if got := s.Terminal(); got != terminal[s] {
			t.Errorf("%s.Terminal() = %v, want %v", s, got, terminal[s])
		}
	}
	if order.Status("bogus").Terminal() {
		t.Error(`Status("bogus").Terminal() = true, want false`)
	}
}
