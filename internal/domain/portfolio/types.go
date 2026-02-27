package portfolio

import (
	"time"

	"github.com/shopspring/decimal"
)

// Position represents a single asset holding within a portfolio.
type Position struct {
	Asset                  string          `json:"asset"`
	Total                  decimal.Decimal `json:"total"`
	Available              decimal.Decimal `json:"available"`
	Locked                 decimal.Decimal `json:"locked"`
	AvailableWithoutBorrow decimal.Decimal `json:"available_without_borrow"`
	Borrow                 decimal.Decimal `json:"borrow"`
	Value                  decimal.Decimal `json:"value"`
}

func NewPosition(asset string, total, available, locked, availableWithoutBorrow, borrow, value decimal.Decimal) Position {
	return Position{
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
func (p Position) IsZero() bool {
	return p.Total.IsZero()
}

// Snapshot captures the complete portfolio state at a moment in time
type Snapshot struct {
	Exchange   string              `json:"exchange"`
	Account    AccountType         `json:"account"`
	Positions  map[string]Position `json:"positions"` // asset to position
	TotalValue decimal.Decimal     `json:"total_value"`
	CapturedAt time.Time           `json:"captured_at"`
}

func NewSnapshot(exchange string, account AccountType, capturedAt time.Time) *Snapshot {
	return &Snapshot{
		Exchange:   exchange,
		Account:    account,
		Positions:  make(map[string]Position),
		CapturedAt: capturedAt,
	}
}

// AddPosition adds a position to the snapshot and updates the total value
func (s *Snapshot) AddPosition(pos Position) {
	s.Positions[pos.Asset] = pos
	s.TotalValue = s.TotalValue.Add(pos.Value)
}

// GetPosition returns the position for a specific asset
func (s *Snapshot) GetPosition(asset string) (Position, bool) {
	pos, ok := s.Positions[asset]
	return pos, ok
}

// NonZeroPositions returns a slice of positions that are non-zero total
func (s *Snapshot) NonZeroPositions() []Position {
	result := make([]Position, 0, len(s.Positions))

	for _, p := range s.Positions {
		if !p.IsZero() {
			result = append(result, p)
		}
	}
	return result
}

// Assets returns a list of all assets in the snapshot
func (s *Snapshot) Assets() []string {
	assets := make([]string, 0, len(s.Positions))

	for asset := range s.Positions {
		assets = append(assets, asset)
	}
	return assets
}
