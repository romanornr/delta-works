package clocktest

import (
	"sync"
	"time"

	"github.com/romanornr/delta-works/internal/clock"
)

// Mock is a clock implementation that for testing with controlled time.
// It is safe for concurrent use.
type Mock struct {
	mu      sync.Mutex
	current time.Time
}

// Ensure Mock implements clock.Clock at compile time.
var _ clock.Clock = (*Mock)(nil)

// New creates a new mock clock set to the given time.
func New(t time.Time) *Mock {
	return &Mock{current: t}
}

// Now returns the mock's current time.
func (m *Mock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// Set updates the mock's current time.
func (m *Mock) Set(t time.Time) {
	m.mu.Lock()
	m.current = t
	m.mu.Unlock()
}

// Add advances the mock's current time by the given duration.
func (m *Mock) Add(d time.Duration) {
	m.mu.Lock()
	m.current = m.current.Add(d)
	m.mu.Unlock()
}
