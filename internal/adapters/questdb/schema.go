// Package questdb provides a QuestDB-backed implementation of the storage
// interfaces.
//
// Writes are performed via QuestDB's ILP HTTP ingestion endpoint.
// Reads are performed via QuestDB's PostgreSQL wire protocol using database/sql.
package questdb

import (
	"context"
	"database/sql"
	"fmt"
)

const createPortfolioSnapshotsTable = `
CREATE TABLE IF NOT EXISTS portfolio_snapshots (
  exchange SYMBOL,
  account SYMBOL,
  total_value DECIMAL(38, 18),
  captured_at TIMESTAMP
) TIMESTAMP(captured_at) PARTITION BY DAY;
`

const createPortfolioPositionsTable = `
CREATE TABLE IF NOT EXISTS portfolio_positions (
  exchange SYMBOL,
  account SYMBOL,
  asset SYMBOL,
  total DECIMAL(38, 18),
  available DECIMAL(38, 18),
  locked DECIMAL(38, 18),
  available_without_borrow DECIMAL(38, 18),
  borrow DECIMAL(38, 18),
  value_usd DECIMAL(38, 18),
  captured_at TIMESTAMP
) TIMESTAMP(captured_at) PARTITION BY DAY;
`

const createTransfersTable = `
CREATE TABLE IF NOT EXISTS transfers (
  exchange SYMBOL,
  direction SYMBOL,
  transfer_type SYMBOL,
  asset SYMBOL,
  amount DECIMAL(38, 18),
  fee DECIMAL(38, 18),
  status SYMBOL,
  network SYMBOL,
  tx_hash STRING,
  address STRING,
  bank_to STRING,
  description STRING,
  completed_at TIMESTAMP
) TIMESTAMP(completed_at) PARTITION BY DAY;
`

// EnsureSchema creates all required QuestDB tables if they do not already exist.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	ddls := []struct {
		name string
		sql  string
	}{
		{name: "portfolio_snapshots", sql: createPortfolioSnapshotsTable},
		{name: "portfolio_positions", sql: createPortfolioPositionsTable},
		{name: "transfers", sql: createTransfersTable},
	}

	for _, ddl := range ddls {
		if _, err := db.ExecContext(ctx, ddl.sql); err != nil {
			return fmt.Errorf("failed to create table %s: %w", ddl.name, err)
		}
	}
	return nil
}
