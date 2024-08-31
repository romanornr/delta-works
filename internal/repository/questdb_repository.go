package repository

import (
	"context"
	"fmt"
	"github.com/questdb/go-questdb-client/v3"
	"github.com/romanornr/delta-works/internal/models"
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

func (q *QuestDBRepository) InsertHoldings(ctx context.Context, holding models.AccountHoldings) error {
	for currency, balance := range holding.Balances {
		err := q.sender.
			Table("holdings").
			Symbol("exchange", holding.ExchangeName).
			Symbol("account_type", holding.AccountType.String()).
			Symbol("symbol", currency.String()).
			Float64Column("total", balance.Total.InexactFloat64()).
			Float64Column("hold", balance.Hold.InexactFloat64()).
			Float64Column("free", balance.Free.InexactFloat64()).
			Float64Column("available_without_borrow", balance.AvailableWithoutBorrow.InexactFloat64()).
			Float64Column("borrowed", balance.Borrowed.InexactFloat64()).
			Float64Column("usd_value", balance.USDValue.InexactFloat64()).
			At(ctx, holding.LastUpdated)
		if err != nil {
			return fmt.Errorf("failed to insert holding data: %v", err)
		}
	}

	// Insert total USD value for the account
	err := q.sender.
		Table("account_totals").
		Symbol("exchange", holding.ExchangeName).
		Symbol("account_type", holding.AccountType.String()).
		Float64Column("total_usd_value", holding.TotalUSDValue.InexactFloat64()).
		At(ctx, holding.LastUpdated)
	if err != nil {
		return fmt.Errorf("failed to insert account total data: %v", err)
	}

	err = q.sender.Flush(ctx)
	if err != nil {
		return fmt.Errorf("failed to flush data: %v", err)
	}

	return nil
}

func (q *QuestDBRepository) Close(ctx context.Context) error {
	return q.sender.Close(ctx)
}
