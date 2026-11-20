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
	"github.com/jackc/pgx/v5/pgtype"

	domaintrip "github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres/transaction"
	"github.com/saifullinartem/saifullin_golang/internal/model"
)

const statusColumn = "status"

type Repository struct {
	pool    executor
	timeout time.Duration
}

func (r *Repository) SavePosition(ctx context.Context, p model.PositionInput) (model.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	exec := transaction.ExecutorFromContext(ctx, r.pool)
	query := `INSERT INTO trip_positions (trip_id, latitude, longitude, recorded_at)
		SELECT id, $2, $3, $4 FROM trips WHERE id = $1 AND status = 'active' FOR NO KEY UPDATE
		RETURNING id, trip_id, latitude, longitude, recorded_at`
	var result model.Position
	err := exec.QueryRow(ctx, query, p.TripID, p.Coordinates.Latitude, p.Coordinates.Longitude, p.RecordedAt).Scan(&result.ID, &result.TripID, &result.Coordinates.Latitude, &result.Coordinates.Longitude, &result.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := r.Get(ctx, p.TripID); getErr != nil {
			return model.Position{}, getErr
		}
		return model.Position{}, domaintrip.ErrTripCompleted
	}
	if err != nil {
		return model.Position{}, fmt.Errorf("save position: %w", err)
	}
	update := `UPDATE trips SET last_position_at = GREATEST(last_position_at, $2) WHERE id = $1`
	if _, err := exec.Exec(ctx, update, p.TripID, p.RecordedAt); err != nil {
		return model.Position{}, fmt.Errorf("update last position: %w", err)
	}
	result.RecordedAt = result.RecordedAt.UTC()
	return result, nil
}

func (r *Repository) ListPositions(ctx context.Context, id uuid.UUID) ([]model.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	if _, err := r.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := transaction.ExecutorFromContext(ctx, r.pool).Query(ctx, `SELECT id, trip_id, latitude, longitude, recorded_at FROM trip_positions WHERE trip_id = $1 ORDER BY recorded_at, id`, id)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	defer rows.Close()
	positions := make([]model.Position, 0)
	for rows.Next() {
		var p model.Position
		if err := rows.Scan(&p.ID, &p.TripID, &p.Coordinates.Latitude, &p.Coordinates.Longitude, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		p.RecordedAt = p.RecordedAt.UTC()
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate positions: %w", err)
	}
	return positions, nil
}

func (r *Repository) StaleTrips(ctx context.Context, after time.Duration) ([]model.Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query := `SELECT ` + strings.Join(tripColumns(), ", ") + ` FROM trips WHERE status = 'active' AND COALESCE(last_position_at, started_at) < now() - $1::interval ORDER BY COALESCE(last_position_at, started_at), id`
	rows, err := transaction.ExecutorFromContext(ctx, r.pool).Query(ctx, query, pgtype.Interval{Microseconds: after.Microseconds(), Valid: true})
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()
	trips := make([]model.Trip, 0)
	for rows.Next() {
		t, err := scanTrip(rows)
		if err != nil {
			return nil, err
		}
		trips = append(trips, t)
	}
	return trips, rows.Err()
}

func (r *Repository) StaleTripsBatch(ctx context.Context, after time.Duration, limit int, cursor *model.StaleCursor) ([]model.Trip, *model.StaleCursor, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	query := `SELECT ` + strings.Join(tripColumns(), ", ") + `, COALESCE(last_position_at, started_at) AS stale_at
		FROM trips
		WHERE status = 'active' AND COALESCE(last_position_at, started_at) < now() - $1::interval
	`
	args := []any{pgtype.Interval{Microseconds: after.Microseconds(), Valid: true}, limit}
	if cursor != nil {
		query += ` AND (COALESCE(last_position_at, started_at), id) > ($3::timestamptz, $4::uuid)`
		args = append(args, cursor.At, cursor.ID)
	}
	query += ` ORDER BY COALESCE(last_position_at, started_at), id LIMIT $2`
	rows, err := transaction.ExecutorFromContext(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("select stale trip batch: %w", err)
	}
	defer rows.Close()
	trips := make([]model.Trip, 0, limit)
	var last time.Time
	for rows.Next() {
		var t model.Trip
		if err := rows.Scan(&t.ID, &t.UserID, &t.DriverID, &t.StartPoint.Latitude, &t.StartPoint.Longitude, &t.EndPoint.Latitude, &t.EndPoint.Longitude, &t.Price, &t.Status, &t.StartedAt, &t.FinishedAt, &t.LastPositionAt, &last); err != nil {
			return nil, nil, fmt.Errorf("scan stale trip batch: %w", err)
		}
		trips = append(trips, t)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(trips) == 0 {
		return trips, nil, nil
	}
	return trips, &model.StaleCursor{At: last, ID: trips[len(trips)-1].ID}, nil
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

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (model.Trip, error) { //nolint:unparam
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
	return []string{"id", "user_id", "driver_id", "start_latitude", "start_longitude", "end_latitude", "end_longitude", "price", statusColumn, "started_at", "finished_at", "last_position_at"}
}

func scanTrip(row pgx.Row) (model.Trip, error) {
	var trip model.Trip
	err := row.Scan(&trip.ID, &trip.UserID, &trip.DriverID, &trip.StartPoint.Latitude, &trip.StartPoint.Longitude, &trip.EndPoint.Latitude, &trip.EndPoint.Longitude, &trip.Price, &trip.Status, &trip.StartedAt, &trip.FinishedAt, &trip.LastPositionAt)
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
