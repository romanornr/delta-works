package questdb

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/romanornr/delta-works/internal/domain/transfer"
	"github.com/shopspring/decimal"
)

func TestTransferStoreRangeScansStatusAndDescription(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(context.Background(), `
CREATE TABLE transfers (
	exchange TEXT,
	direction TEXT,
	transfer_type TEXT,
	asset TEXT,
	amount TEXT,
	fee TEXT,
	status TEXT,
	network TEXT,
	tx_hash TEXT,
	address TEXT,
	bank_to TEXT,
	description TEXT,
	completed_at TIMESTAMP
);`)
	if err != nil {
		t.Fatalf("failed to create transfers table: %v", err)
	}

	completedAt := time.Date(2026, 6, 30, 12, 34, 56, 0, time.UTC)
	_, err = db.ExecContext(context.Background(), `
INSERT INTO transfers (
	exchange, direction, transfer_type, asset, amount, fee, status,
	network, tx_hash, address, bank_to, description, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		"bybit",
		string(transfer.Outbound),
		string(transfer.TypeFiat),
		"USD",
		"125.50",
		"0.75",
		string(transfer.Completed),
		"wire-network",
		"tx-123",
		"external-address",
		"recipient-bank",
		"quarterly treasury sweep",
		completedAt,
	)
	if err != nil {
		t.Fatalf("failed to insert transfer row: %v", err)
	}

	store := &TransferStore{client: &Client{db: db}}

	got, err := store.Range(context.Background(), "bybit", completedAt.Add(-time.Hour), completedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("Range returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(got))
	}

	tr := got[0]
	if tr.Exchange != "bybit" {
		t.Fatalf("expected exchange bybit, got %s", tr.Exchange)
	}
	if tr.Direction != transfer.Outbound {
		t.Fatalf("expected direction outbound, got %s", tr.Direction)
	}
	if tr.Type != transfer.TypeFiat {
		t.Fatalf("expected type fiat, got %s", tr.Type)
	}
	if tr.Asset != "USD" {
		t.Fatalf("expected asset USD, got %s", tr.Asset)
	}
	if !tr.Amount.Equal(decimal.RequireFromString("125.50")) {
		t.Fatalf("expected amount 125.50, got %s", tr.Amount)
	}
	if !tr.Fee.Equal(decimal.RequireFromString("0.75")) {
		t.Fatalf("expected fee 0.75, got %s", tr.Fee)
	}
	if tr.Status != transfer.Completed {
		t.Fatalf("expected status completed, got %s", tr.Status)
	}
	if tr.Description != "quarterly treasury sweep" {
		t.Fatalf("expected description to be scanned, got %q", tr.Description)
	}
	if tr.BankTo != "recipient-bank" {
		t.Fatalf("expected bank_to recipient-bank, got %q", tr.BankTo)
	}
}
