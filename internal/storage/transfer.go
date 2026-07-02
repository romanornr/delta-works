package storage

import (
	"context"
	"time"

	"github.com/romanornr/delta-works/internal/domain/transfer"
)

// TransferWriter defines write operations for transfer
type TransferWriter interface {
	Write(ctx context.Context, t transfer.Transfer) error
	WriteBatch(ctx context.Context, batch []transfer.Transfer) error
}

// TransferReader defines read operations for transfers
type TransferReader interface {
	LastTime(ctx context.Context, exchange string, direction transfer.Direction) (time.Time, error)
	Range(ctx context.Context, exchange string, from, to time.Time) ([]transfer.Transfer, error)
}

// TransferStore combines read and write operations
type TransferStore interface {
	TransferWriter
	TransferReader
}
