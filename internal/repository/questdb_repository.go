package repository

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/questdb/go-questdb-client/v3"
	"github.com/romanornr/delta-works/internal/contracts"
)

// QuestDBRepository is a repository for managing data storage in QuestDB.
// It uses questdb.LineSender to send data to the QuestDB instance.
type QuestDBRepository struct {
	sender questdb.LineSender
	db     *sql.DB
	logger contracts.Logger
}

// NewQuestDBRepository creates a new QuestDBRepository instance using the provided context and configuration string.
func NewQuestDBRepository(ctx context.Context, config string, logger contracts.Logger) (*QuestDBRepository, error) {
	sender, err := questdb.LineSenderFromConf(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create QuestDB sender: %w", err)
	}

	connStr := "host=localhost port=8812 user=admin password=quest dbname=qdb sslmode=disable"

	// Establish PostgreSQL connection (for meta operations, if needed)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Validate PostgreSQL connection
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &QuestDBRepository{
		sender: sender,
		db:     db,
		logger: logger,
	}, nil
}

// Close terminates the connection to the QuestDB instance and releases associated resources.
func (q *QuestDBRepository) Close(ctx context.Context) error {
	return q.sender.Close(ctx)
}
