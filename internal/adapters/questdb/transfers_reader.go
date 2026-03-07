package questdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/romanornr/delta-works/internal/domain/transfer"
	"github.com/romanornr/delta-works/internal/errs"
)

// LastTime returns the most recent completed_at for a given exchange and direction
func (s *TransferStore) LastTime(ctx context.Context, exchange string, direction transfer.Direction) (time.Time, error) {
	var ts sql.NullTime
	row := s.client.db.QueryRowContext(
		ctx,
		`SELECT max(completed_at) FROM transfers WHERE exchange = $1 AND direction = $2`,
		exchange,
		string(direction),
	)

	if err := row.Scan(&ts); err != nil {
		return time.Time{}, fmt.Errorf("failed to query last transfer time for exchange %s and direction %s: %w", exchange, string(direction), err)
	}

	if !ts.Valid {
		return time.Time{}, errs.ErrNoTransfersFound
	}

	return ts.Time, nil
}

// Range returns transfers in [from, to].
func (s *TransferStore) Range(ctx context.Context, exchange string, from, to time.Time) ([]transfer.Transfer, error) {
	rows, err := s.client.db.QueryContext(ctx,
		`SELECT exchange, direction, transfer_type, asset, amount, fee, status, network, tx_hash, address, bank_to, description, completed_at
FROM transfers
WHERE exchange = $1 AND completed_at >= $2 AND completed_at <= $3
ORDER BY completed_at`,
		exchange,
		from,
		to,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query transfers for exchange %s from %s to %s: %w", exchange, from.Format(time.RFC3339), to.Format(time.RFC3339), err)
	}
	defer rows.Close()

	var out []transfer.Transfer
	for rows.Next() {
		var t transfer.Transfer
		var direction, typ, status string

		if err := rows.Scan(&t.Exchange, &direction, &typ, &t.Asset, &t.Amount, &t.Fee, &status, &t.Network, &t.TxHash, &t.Address, &t.BankTo, &t.Direction, &t.Description, &t.CompletedAt); err != nil {
			return nil, fmt.Errorf("failed to scan transfer row for exchange %s from %s to %s: %w", exchange, from.Format(time.RFC3339), to.Format(time.RFC3339), err)
		}
		t.Direction = transfer.Direction(direction)
		t.Type = transfer.Type(typ)
		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transfers for exchange %s from %s to %s: %w", exchange, from.Format(time.RFC3339), to.Format(time.RFC3339), err)
	}

	if len(out) == 0 {
		return nil, errs.ErrNoTransfersFound
	}

	return out, nil
}
