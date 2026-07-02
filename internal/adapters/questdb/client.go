// Package questdb provides a QuestDB-backed implementation of the storage interfaces.
//
// Writes are performed via QuestDB's ILP HTTP ingestion endpoint.
// Reads are performed via QuestDB's PostgreSQL wire protocol using database/sql.
package questdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	qdb "github.com/questdb/go-questdb-client/v4"
	"github.com/romanornr/delta-works/internal/config"
	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// Client holds QuestDB connection state.
type Client struct {
	sender qdb.LineSender
	db     *sql.DB
	log    zerolog.Logger
}

// NewClient returns a QuestDB client.
//
// Important: do NOT call EnsureSchema here yet. This keeps the walking-skeleton
// checkpoints explicit (Ping first, then schema, then write, then read).
func NewClient(lc fx.Lifecycle, cfg *config.Config, log zerolog.Logger) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sender, err := qdb.LineSenderFromConf(ctx, cfg.QuestDB.LineSenderURI())
	if err != nil {
		return nil, fmt.Errorf("failed to create questdb line sender: %w", err)
	}

	db, err := sql.Open("postgres", cfg.QuestDB.PostgresConnStr())
	if err != nil {
		_ = sender.Close(ctx)
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	c := &Client{
		sender: sender,
		db:     db,
		log:    log.With().Str("component", "questdb").Logger(),
	}

	// Verify the postgres connection is reachable before proceeding.
	if err := c.Ping(ctx); err != nil {
		_ = c.Close(ctx)
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			return c.Close(ctx)
		},
	})

	return c, nil
}

// DB returns the underlying Postgres connection.
//
// This is intended for bootstrap/checkpoint code (for example EnsureSchema).
func (c *Client) DB() *sql.DB {
	return c.db
}

// Ping verifies the PostgreSQL wire protocol connection is reachable.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping questdb postgres: %w", err)
	}
	return nil
}

// Close flushes any pending ILP rows and closes the sender and DB connections.
// QuestDB's docs recommend explicitly flushing before closing to ensure retries are applied consistently
func (c *Client) Close(ctx context.Context) error {
	var closeErrs []error

	if err := c.sender.Flush(ctx); err != nil {
		c.log.Error().Err(err).Msg("failed to flush questdb line sender")
		closeErrs = append(closeErrs, fmt.Errorf("failed to flush questdb line sender: %w", err))
	}

	if err := c.sender.Close(ctx); err != nil {
		c.log.Error().Err(err).Msg("failed to close questdb line sender")
		closeErrs = append(closeErrs, fmt.Errorf("failed to close questdb line sender: %w", err))
	}

	if err := c.db.Close(); err != nil {
		closeErrs = append(closeErrs, fmt.Errorf("failed to close questdb postgres connection: %w", err))
	}

	return errors.Join(closeErrs...)
}
