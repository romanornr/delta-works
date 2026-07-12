package ledger_test

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"

	"github.com/romanornr/delta-works/internal/domain/ledger"
)

var openedAt = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

func lot(id, remaining string, at time.Time) ledger.Lot {
	qty := decimal.RequireFromString(remaining)
	return ledger.Lot{ID: id, Qty: qty, RemainingQty: qty, OpenedAt: at}
}

func TestFIFOSelect(t *testing.T) {
	tests := []struct {
		name string
		lots []ledger.Lot
		sell string
		want ledger.Allocation
	}{
		{
			name: "exact match", lots: []ledger.Lot{lot("a", "2", openedAt)}, sell: "2",
			want: ledger.Allocation{Closures: []ledger.Closure{{LotID: "a", Qty: decimal.NewFromInt(2)}}},
		},
		{
			name: "partial across lots",
			lots: []ledger.Lot{lot("a", "2", openedAt), lot("b", "3", openedAt.Add(time.Minute))}, sell: "4",
			want: ledger.Allocation{Closures: []ledger.Closure{
				{LotID: "a", Qty: decimal.NewFromInt(2)},
				{LotID: "b", Qty: decimal.NewFromInt(2)},
			}},
		},
		{
			name: "full oversell", sell: "3",
			want: ledger.Allocation{Unmatched: decimal.NewFromInt(3)},
		},
		{
			name: "partial oversell", lots: []ledger.Lot{lot("a", "2", openedAt)}, sell: "3",
			want: ledger.Allocation{
				Closures:  []ledger.Closure{{LotID: "a", Qty: decimal.NewFromInt(2)}},
				Unmatched: decimal.NewFromInt(1),
			},
		},
		{
			name: "zero remaining skipped",
			lots: []ledger.Lot{lot("a", "0", openedAt), lot("b", "2", openedAt.Add(time.Minute))}, sell: "1",
			want: ledger.Allocation{Closures: []ledger.Closure{{LotID: "b", Qty: decimal.NewFromInt(1)}}},
		},
		{
			name: "lot ID breaks timestamp tie",
			lots: []ledger.Lot{lot("b", "1", openedAt), lot("a", "1", openedAt)}, sell: "1",
			want: ledger.Allocation{Closures: []ledger.Closure{{LotID: "a", Qty: decimal.NewFromInt(1)}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := append([]ledger.Lot(nil), tt.lots...)
			got := (ledger.FIFO{}).Select(tt.lots, decimal.RequireFromString(tt.sell))
			if !equalAllocation(got, tt.want) {
				t.Fatalf("Select = %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.lots, before) {
				t.Fatalf("input mutated: got %+v, want %+v", tt.lots, before)
			}
		})
	}
}

func equalAllocation(a, b ledger.Allocation) bool {
	if len(a.Closures) != len(b.Closures) || !a.Unmatched.Equal(b.Unmatched) {
		return false
	}
	for i := range a.Closures {
		if a.Closures[i].LotID != b.Closures[i].LotID || !a.Closures[i].Qty.Equal(b.Closures[i].Qty) {
			return false
		}
	}
	return true
}

func TestPropFIFOConservesQuantityAndOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		remaining := rapid.SliceOfN(rapid.IntRange(0, 500), 0, 20).Draw(t, "remaining")
		opened := rapid.SliceOfN(rapid.IntRange(0, 10), len(remaining), len(remaining)).Draw(t, "opened")
		lots := make([]ledger.Lot, len(remaining))
		for i := range remaining {
			lots[i] = lot(
				fmt.Sprintf("lot-%03d", i),
				decimal.New(int64(remaining[i]), -2).String(),
				openedAt.Add(time.Duration(opened[i])*time.Second),
			)
		}
		sellQty := decimal.New(int64(rapid.IntRange(1, 1000).Draw(t, "sell_qty")), -2)
		allocation := (ledger.FIFO{}).Select(lots, sellQty)

		sum := allocation.Unmatched
		byID := make(map[string]ledger.Lot, len(lots))
		eligible := make([]ledger.Lot, 0, len(lots))
		for _, candidate := range lots {
			byID[candidate.ID] = candidate
			if candidate.RemainingQty.IsPositive() {
				eligible = append(eligible, candidate)
			}
		}
		sort.Slice(eligible, func(i, j int) bool {
			if eligible[i].OpenedAt.Equal(eligible[j].OpenedAt) {
				return eligible[i].ID < eligible[j].ID
			}
			return eligible[i].OpenedAt.Before(eligible[j].OpenedAt)
		})
		for i, closure := range allocation.Closures {
			candidate, ok := byID[closure.LotID]
			if !ok || !closure.Qty.IsPositive() || closure.Qty.GreaterThan(candidate.RemainingQty) {
				t.Fatalf("invalid closure %+v for lot %+v", closure, candidate)
			}
			if i >= len(eligible) {
				t.Fatalf("closure %d = %s has no eligible FIFO lot", i, closure.LotID)
			}
			if closure.LotID != eligible[i].ID {
				t.Fatalf("closure %d = %s, want FIFO lot %s", i, closure.LotID, eligible[i].ID)
			}
			sum = sum.Add(closure.Qty)
		}
		if !sum.Equal(sellQty) {
			t.Fatalf("closures + unmatched = %s, want %s", sum, sellQty)
		}
	})
}
