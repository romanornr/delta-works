package repository

import (
	"context"
	"fmt"
	"github.com/questdb/go-questdb-client/v3"
)

// QuestDBRepository is a repository for managing data storage in QuestDB.
// It uses questdb.LineSender to send data to the QuestDB instance.
type QuestDBRepository struct {
	sender questdb.LineSender
}

// NewQuestDBRepository creates a new QuestDBRepository instance using the provided context and configuration string.
func NewQuestDBRepository(ctx context.Context, config string) (*QuestDBRepository, error) {
	sender, err := questdb.LineSenderFromConf(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create QuestDB sender: %w", err)
	}

	return &QuestDBRepository{
		sender: sender,
	}, nil
}

func (q *QuestDBRepository) Close(ctx context.Context) error {
	return q.sender.Close(ctx)
}
