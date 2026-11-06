package trip

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domaintrip "github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres/transaction"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

const statusColumn = "status"

type Repository struct {
	pool    executor
	timeout time.Duration
}

func NewRepository(pool executor, timeout time.Duration) *Repository {
	return &Repository{pool: pool, timeout: timeout}
}

func (r *Repository) Create(ctx context.Context, trip model.Trip) error {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Insert("trips").
		Columns("id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", statusColumn, "started_at", "finished_at").
		Values(trip.ID, trip.UserID, trip.DriverID, trip.StartPoint.Latitude, trip.StartPoint.Longitude, trip.EndPoint.Latitude, trip.EndPoint.Longitude, trip.Price, trip.Status, trip.StartedAt, trip.FinishedAt).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return fmt.Errorf("build create trip query: %w", err)
	}
	if _, err := transaction.ExecutorFromContext(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "trips_active_driver_idx" {
			return domaintrip.ErrDriverBusy
		}
		return fmt.Errorf("create trip: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Select(tripColumns()...).From("trips").Where(sq.Eq{"id": id}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}
	trip, err := scanTrip(transaction.ExecutorFromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Trip{}, domaintrip.ErrTripNotFound
	}
	if err != nil {
		return model.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

func (r *Repository) Finish(ctx context.Context, id uuid.UUID, finishedAt time.Time) (model.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Update("trips").
		Set(statusColumn, domaintrip.Completed).Set("finished_at", finishedAt).Set("updated_at", finishedAt).
		Where(sq.Eq{"id": id, statusColumn: domaintrip.Active}).
		Suffix("RETURNING " + strings.Join(tripColumns(), ", ")).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return model.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}
	trip, err := scanTrip(transaction.ExecutorFromContext(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, readErr := r.Get(ctx, id); readErr != nil {
			return model.Trip{}, readErr
		}
		return model.Trip{}, domaintrip.ErrTripCompleted
	}
	if err != nil {
		return model.Trip{}, fmt.Errorf("finish trip: %w", err)
	}
	return trip, nil
}

func tripColumns() []string {
	return []string{"id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", statusColumn, "started_at", "finished_at"}
}

func scanTrip(row pgx.Row) (model.Trip, error) {
	var trip model.Trip
	err := row.Scan(&trip.ID, &trip.UserID, &trip.DriverID, &trip.StartPoint.Latitude, &trip.StartPoint.Longitude, &trip.EndPoint.Latitude, &trip.EndPoint.Longitude, &trip.Price, &trip.Status, &trip.StartedAt, &trip.FinishedAt)
	if err == nil {
		trip.StartedAt = trip.StartedAt.UTC()
		if trip.FinishedAt != nil {
			finishedAt := trip.FinishedAt.UTC()
			trip.FinishedAt = &finishedAt
		}
	}
	return trip, err
}

func (r *Repository) CountActive(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query, args, err := sq.Select("count(*)").From("trips").Where(sq.Eq{statusColumn: domaintrip.Active}).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}
	var count int64
	if err := transaction.ExecutorFromContext(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}
	return count, nil
}
