package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saifullinartem/saifullin_golang/internal/database"
)

type HistoryRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewHistoryRepository(pool *pgxpool.Pool, timeout time.Duration) *HistoryRepository {
	return &HistoryRepository{pool: pool, timeout: timeout}
}

func (r *HistoryRepository) Add(ctx context.Context, change StatusChange) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(change.TripID, change.FromStatus, change.ToStatus, change.Reason, change.ChangedAt).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build status history query: %w", err)
	}
	if _, err := database.ExecutorFromContext(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("add status history: %w", err)
	}
	return nil
}
