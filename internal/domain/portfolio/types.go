package portfolio

import (
	"time"

	"github.com/shopspring/decimal"
)

// Holding represents a single asset holding within a portfolio.
type Holding struct {
	Asset                  string          `json:"asset"`
	Total                  decimal.Decimal `json:"total"`
	Available              decimal.Decimal `json:"available"`
	Locked                 decimal.Decimal `json:"locked"`
	AvailableWithoutBorrow decimal.Decimal `json:"available_without_borrow"`
	Borrow                 decimal.Decimal `json:"borrow"`
	Value                  decimal.Decimal `json:"value"`
}

func NewHolding(asset string, total, available, locked, availableWithoutBorrow, borrow, value decimal.Decimal) Holding {
	return Holding{
		Asset:                  asset,
		Total:                  total,
		Available:              available,
		Locked:                 locked,
		AvailableWithoutBorrow: availableWithoutBorrow,
		Borrow:                 borrow,
		Value:                  value,
	}
}

// IsZero reports whether the total balance is zero
func (h Holding) IsZero() bool {
	return h.Total.IsZero()
}

// Snapshot captures the complete portfolio state at a moment in time
type Snapshot struct {
	Exchange   string             `json:"exchange"`
	Account    AccountType        `json:"account"`
	Holdings   map[string]Holding `json:"holdings"` // asset to holding
	TotalValue decimal.Decimal    `json:"total_value"`
	CapturedAt time.Time          `json:"captured_at"`
}

func NewSnapshot(exchange string, account AccountType, capturedAt time.Time) *Snapshot {
	return &Snapshot{
		Exchange:   exchange,
		Account:    account,
		Holdings:   make(map[string]Holding),
		CapturedAt: capturedAt,
	}
}

// AddHolding adds a holding to the snapshot and updates the total value
func (s *Snapshot) AddHolding(holding Holding) {
	if existing, ok := s.Holdings[holding.Asset]; ok {
		s.TotalValue = s.TotalValue.Sub(existing.Value)
	}
	s.Holdings[holding.Asset] = holding
	s.TotalValue = s.TotalValue.Add(holding.Value)
}

// GetHolding returns the holding for a specific asset
func (s *Snapshot) GetHolding(asset string) (Holding, bool) {
	holding, ok := s.Holdings[asset]
	return holding, ok
}

// NonZeroHoldings returns a slice of holdings that are non-zero total
func (s *Snapshot) NonZeroHoldings() []Holding {
	result := make([]Holding, 0, len(s.Holdings))

	for _, holding := range s.Holdings {
		if !holding.IsZero() {
			result = append(result, holding)
		}
	}
	return result
}

// Assets returns a list of all assets in the snapshot
func (s *Snapshot) Assets() []string {
	assets := make([]string, 0, len(s.Holdings))

	for asset := range s.Holdings {
		assets = append(assets, asset)
	}
	return assets
}
