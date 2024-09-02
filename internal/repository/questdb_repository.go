package repository

import (
	"context"
	"fmt"
	"github.com/questdb/go-questdb-client/v3"
)

type QuestDBRepository struct {
	sender questdb.LineSender
}

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
