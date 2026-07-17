// Package snapshot owns balance-snapshot checkpoint models.
package snapshot

import (
	"time"

	"github.com/google/uuid"

	"github.com/romanornr/delta-works/internal/domain/account"
)

// Checkpoint is the durable record that a balance snapshot reached the
// time-series store. It is the Postgres-side anchor for gap detection.
type Checkpoint struct {
	ID           uuid.UUID
	Account      account.Ref
	TakenAt      time.Time
	BalanceCount int
	Status       Status
	Error        string
}

// Status classifies a snapshot attempt.
type Status string

// Checkpoint status values.
const (
	StatusOK     Status = "ok"
	StatusFailed Status = "failed"
)
