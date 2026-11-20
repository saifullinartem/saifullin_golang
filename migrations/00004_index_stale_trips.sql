-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY IF NOT EXISTS trips_stale_positions_idx
    ON trips (status, (COALESCE(last_position_at, started_at)), id);

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS trips_stale_positions_idx;
