// Package questdb provides a QuestDB-backed implementation of the storage interfaces
package questdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/romanornr/delta-works/internal/domain/portfolio"
	"github.com/romanornr/delta-works/internal/errs"
)

// Latest returns the most recent snapshot for the given exchange and account
// If no snapshot exists, it returns errs.ErrNoSnapshotsFound
func (s *SnapshotStore) Latest(ctx context.Context, exchange string, account portfolio.AccountType) (*portfolio.Snapshot, error) {
	var capturedAt sql.NullTime

	row := s.client.db.QueryRowContext(ctx, `SELECT max(captured_at) FROM portfolio_snapshots WHERE exchange = $1 and account = $2`, exchange, account.String())

	if err := row.Scan(&capturedAt); err != nil {
		return nil, fmt.Errorf("failed to query latest snapshot timestamp: exchange=%s, account=%s: %w", exchange, account.String(), err)
	}

	if !capturedAt.Valid || capturedAt.Time.IsZero() {
		return nil, errs.ErrNoSnapshotsFound
	}

	return s.loadSnapshotAt(ctx, exchange, account, capturedAt.Time)
}

// Range returns all snapshots for the given exchange and account within the inclusive time range [from, to]
// Returns errs.ErrNoSnapshotsFound if none exists
func (s *SnapshotStore) Range(ctx context.Context, exchange string, account portfolio.AccountType, from, to time.Time) ([]portfolio.Snapshot, error) {
	rows, err := s.client.db.QueryContext(
		ctx,
		`SELECT captured_at FROM portfolio_snapshots WHERE exchange = $1 AND account = $2 AND captured_at >= $3 AND captured_at <= $4 ORDER BY captured_at`,
		exchange,
		account.String(),
		from,
		to,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshots range [exchange %s account %s]: %w", exchange, account.String(), err)
	}
	defer func() { _ = rows.Close() }()

	var out []portfolio.Snapshot
	for rows.Next() {
		var ts time.Time
		if err := rows.Scan(&ts); err != nil {
			return nil, fmt.Errorf("failed to scan snapshot timestamp [exchange %s account %s]: %w", exchange, account.String(), err)
		}

		snap, err := s.loadSnapshotAt(ctx, exchange, account, ts)
		if err != nil {
			return nil, err
		}
		out = append(out, *snap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate snapshot timestamps [exchange %s account %s]: %w", exchange, account.String(), err)
	}

	if len(out) == 0 {
		return nil, errs.ErrNoSnapshotsFound
	}
	return out, nil
}

func (s *SnapshotStore) loadSnapshotAt(ctx context.Context, exchange string, account portfolio.AccountType, capturedAt time.Time) (*portfolio.Snapshot, error) {
	rows, err := s.client.db.QueryContext(
		ctx,
		`SELECT asset, total, available, locked, available_without_borrow, borrow, value_usd
FROM portfolio_positions
WHERE exchange = $1 AND account = $2 AND captured_at = $3`,
		exchange,
		account.String(),
		capturedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshot holdings [exchange=%s account=%s captured_at=%s]: %w", exchange, account.String(), capturedAt.Format(time.RFC3339Nano), err)
	}
	defer func() { _ = rows.Close() }()

	snap := portfolio.NewSnapshot(exchange, account, capturedAt)

	for rows.Next() {
		var asset string
		var total, available, locked, awb, borrow, value sql.NullString
		if err := rows.Scan(&asset, &total, &available, &locked, &awb, &borrow, &value); err != nil {
			return nil, fmt.Errorf("failed to scan holding row [exchange=%s account=%s captured_at=%s]: %w", exchange, account.String(), capturedAt.Format(time.RFC3339Nano), err)
		}

		pos := portfolio.Holding{Asset: asset}
		if total.Valid {
			pos.Total, err = decimal.NewFromString(total.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse total decimal [asset=%s]: %w", asset, err)
			}
		}
		if available.Valid {
			pos.Available, err = decimal.NewFromString(available.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse available decimal [asset=%s]: %w", asset, err)
			}
		}
		if locked.Valid {
			pos.Locked, err = decimal.NewFromString(locked.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse locked decimal [asset=%s]: %w", asset, err)
			}
		}
		if awb.Valid {
			pos.AvailableWithoutBorrow, err = decimal.NewFromString(awb.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse available_without_borrow decimal [asset=%s]: %w", asset, err)
			}
		}
		if borrow.Valid {
			pos.Borrow, err = decimal.NewFromString(borrow.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse borrow decimal [asset=%s]: %w", asset, err)
			}
		}
		if value.Valid {
			pos.Value, err = decimal.NewFromString(value.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse value decimal [asset=%s]: %w", asset, err)
			}
		}

		snap.AddHolding(pos)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate snapshot holdings [exchange=%s account=%s captured_at=%s]: %w", exchange, account.String(), capturedAt.Format(time.RFC3339Nano), err)
	}

	if len(snap.Holdings) == 0 {
		return nil, errs.ErrNoSnapshotsFound
	}

	return snap, nil
}
