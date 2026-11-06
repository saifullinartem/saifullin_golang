package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres/transaction"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

type HistoryRepository struct {
	pool    executor
	timeout time.Duration
}

func NewHistoryRepository(pool executor, timeout time.Duration) *HistoryRepository {
	return &HistoryRepository{pool: pool, timeout: timeout}
}

func (r *HistoryRepository) Add(ctx context.Context, change model.StatusChange) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(change.TripID, change.FromStatus, change.ToStatus, change.Reason, change.ChangedAt).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build status history query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("add status history: %w", err)
	}
	return nil
}
