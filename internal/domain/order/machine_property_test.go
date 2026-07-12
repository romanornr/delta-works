package order_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"

	"github.com/romanornr/delta-works/internal/domain/order"
)

var propertyStatuses = []order.Status{
	order.StatusPending,
	order.StatusOpen,
	order.StatusPartiallyFilled,
	order.StatusFilled,
	order.StatusCanceled,
	order.StatusRejected,
	order.StatusExpired,
}

var nonTerminalStatuses = []order.Status{
	order.StatusPending,
	order.StatusOpen,
	order.StatusPartiallyFilled,
}

var terminalStatuses = []order.Status{
	order.StatusFilled,
	order.StatusCanceled,
	order.StatusRejected,
	order.StatusExpired,
}

var filledQtyGen = rapid.Map(rapid.IntRange(0, 500), func(n int) decimal.Decimal {
	return decimal.New(int64(n), -2)
})

var (
	statusGen           = rapid.SampledFrom(propertyStatuses)
	eventGen            = newEventGen(propertyStatuses)
	nonTerminalEventGen = newEventGen(nonTerminalStatuses)
	terminalEventGen    = newEventGen(terminalStatuses)
	eventSequenceGen    = rapid.SliceOfN(eventGen, 1, 40)
	stateGen            = rapid.Custom(func(t *rapid.T) order.State {
		return order.State{
			Status:    statusGen.Draw(t, "status"),
			FilledQty: filledQtyGen.Draw(t, "filled_qty"),
		}
	})
)

func newEventGen(statuses []order.Status) *rapid.Generator[order.Event] {
	statusesGen := rapid.SampledFrom(statuses)
	return rapid.Custom(func(t *rapid.T) order.Event {
		return order.Event{
			Status:    statusesGen.Draw(t, "status"),
			FilledQty: filledQtyGen.Draw(t, "filled_qty"),
		}
	})
}

// applyEvent mirrors the store fold: status transitions conditionally,
// positive fill deltas apply independently, and regressions never un-fill.
func applyEvent(t *rapid.T, state order.State, event order.Event) (order.State, order.Decision) {
	decision, err := order.Transition(state, event)
	if err != nil {
		t.Fatalf("Transition(%+v, %+v): %v", state, event, err)
	}
	if decision.Transition {
		state.Status = decision.To
	}
	state.FilledQty = state.FilledQty.Add(decision.FillDelta)
	return state, decision
}

func foldEvents(t *rapid.T, events []order.Event) order.State {
	state := order.State{Status: order.StatusPending, FilledQty: decimal.Zero}
	for _, event := range events {
		state, _ = applyEvent(t, state, event)
	}
	return state
}

func specRank(status order.Status) int {
	switch status {
	case order.StatusPending:
		return 0
	case order.StatusOpen:
		return 1
	case order.StatusPartiallyFilled:
		return 2
	case order.StatusFilled, order.StatusCanceled, order.StatusRejected, order.StatusExpired:
		return 3
	default:
		panic("invalid generated status")
	}
}

func TestPropTransitionTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		state := stateGen.Draw(t, "state")
		event := eventGen.Draw(t, "event")
		decision, err := order.Transition(state, event)
		if err != nil {
			t.Fatalf("Transition(%+v, %+v): %v", state, event, err)
		}
		if decision.FillDelta.IsNegative() {
			t.Fatalf("negative FillDelta: state=%+v event=%+v decision=%+v", state, event, decision)
		}
		if decision.FillAnomaly && !decision.FillDelta.IsZero() {
			t.Fatalf("fill anomaly carries a delta: state=%+v event=%+v decision=%+v", state, event, decision)
		}
		if decision.Drop == "" && !decision.Transition && !decision.FillDelta.IsPositive() {
			t.Fatalf("incoherent decision without transition, fill, or drop: %+v", decision)
		}
		if decision.Transition && decision.Drop != "" {
			t.Fatalf("transition carries drop reason: %+v", decision)
		}
	})
}

func TestPropRankMonotonic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := eventSequenceGen.Draw(t, "events")
		state := order.State{Status: order.StatusPending, FilledQty: decimal.Zero}
		for i, event := range events {
			before := specRank(state.Status)
			state, _ = applyEvent(t, state, event)
			if after := specRank(state.Status); after < before {
				t.Fatalf("rank decreased at event %d: before=%d after=%d event=%+v", i, before, after, event)
			}
		}
	})
}

func TestPropFilledQtyMonotonic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := eventSequenceGen.Draw(t, "events")
		state := order.State{Status: order.StatusPending, FilledQty: decimal.Zero}
		for i, event := range events {
			before := state.FilledQty
			state, _ = applyEvent(t, state, event)
			if state.FilledQty.LessThan(before) {
				t.Fatalf("filled quantity decreased at event %d: before=%s after=%s event=%+v", i, before, state.FilledQty, event)
			}
		}
	})
}

func TestPropIdempotent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		state := foldEvents(t, eventSequenceGen.Draw(t, "prefix"))
		event := eventGen.Draw(t, "event")
		once, _ := applyEvent(t, state, event)
		twice, second := applyEvent(t, once, event)
		if twice.Status != once.Status || !twice.FilledQty.Equal(once.FilledQty) {
			t.Fatalf("second application changed state: once=%+v twice=%+v event=%+v", once, twice, event)
		}
		if second.Transition || !second.FillDelta.IsZero() {
			t.Fatalf("second application was not a no-op: state=%+v event=%+v decision=%+v", once, event, second)
		}
	})
}

func TestPropPermutationInvariant(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := rapid.SliceOfN(nonTerminalEventGen, 1, 39).Draw(t, "events")
		if rapid.Bool().Draw(t, "with_terminal") {
			events = append(events, terminalEventGen.Draw(t, "terminal"))
		}
		first := foldEvents(t, rapid.Permutation(events).Draw(t, "first_permutation"))
		second := foldEvents(t, rapid.Permutation(events).Draw(t, "second_permutation"))
		if first.Status != second.Status || !first.FilledQty.Equal(second.FilledQty) {
			t.Fatalf("permutations diverged: first=%+v second=%+v events=%+v", first, second, events)
		}
	})
}

func TestPropPendingNeverApplies(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		state := stateGen.Draw(t, "state")
		event := order.Event{
			Status:    order.StatusPending,
			FilledQty: filledQtyGen.Draw(t, "filled_qty"),
		}
		decision, err := order.Transition(state, event)
		if err != nil {
			t.Fatalf("Transition(%+v, %+v): %v", state, event, err)
		}
		if decision.Transition || !decision.FillDelta.IsZero() {
			t.Fatalf("pending event applied: state=%+v event=%+v decision=%+v", state, event, decision)
		}
	})
}

func TestPropFilledQtyEqualsMaxSeen(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		events := eventSequenceGen.Draw(t, "events")
		state := foldEvents(t, events)
		maxSeen := decimal.Zero
		// The spec extracts every positive cumulative delta from a non-pending
		// event, including dropped events. Pending events are the sole exception.
		for _, event := range events {
			if event.Status != order.StatusPending && event.FilledQty.GreaterThan(maxSeen) {
				maxSeen = event.FilledQty
			}
		}
		if !state.FilledQty.Equal(maxSeen) {
			t.Fatalf("FilledQty = %s, want max seen %s for events %+v", state.FilledQty, maxSeen, events)
		}
	})
}
