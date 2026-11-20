//go:build integration

package trip_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/saifullinartem/saifullin_golang/internal/domain/trip"
	"github.com/saifullinartem/saifullin_golang/internal/infra/postgres/transaction"
	"github.com/saifullinartem/saifullin_golang/internal/model"
	repository "github.com/saifullinartem/saifullin_golang/internal/repository/trip"
)

func setup(t *testing.T) (context.Context, *pgxpool.Pool, *repository.Repository, *repository.HistoryRepository, *transaction.TxManager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required; run tripgoctl environment start and make test-integration")
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, dropErr := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); dropErr != nil {
			t.Error(dropErr)
		}
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil || len(files) != 4 {
		t.Fatalf("migration files=%v,error=%v", files, err)
	}
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		up := strings.Split(string(data), "-- +goose Down")[0]
		if _, execErr := pool.Exec(ctx, up); execErr != nil {
			t.Fatal(execErr)
		}
	}
	return ctx, pool, repository.NewRepository(pool, 5*time.Second), repository.NewHistoryRepository(pool, 5*time.Second), transaction.NewTxManager(pool, 5*time.Second)
}

func TestPositionsAndStaleTrips(t *testing.T) {
	ctx, _, repo, _, tx := setup(t)
	old := newTrip()
	old.StartedAt = time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Microsecond)
	fresh := newTrip()
	fresh.StartedAt = time.Now().UTC().Truncate(time.Microsecond)
	completed := newTrip()
	completed.StartedAt = old.StartedAt
	withoutPosition := newTrip()
	withoutPosition.StartedAt = old.StartedAt
	for _, value := range []model.Trip{old, fresh, completed, withoutPosition} {
		if err := repo.Create(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Finish(ctx, completed.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	oldPoint := model.PositionInput{TripID: old.ID, Coordinates: model.Coordinates{Latitude: 59.9, Longitude: 30.3}, RecordedAt: old.StartedAt.Add(2 * time.Minute)}
	newPoint := oldPoint
	newPoint.RecordedAt = old.StartedAt.Add(5 * time.Minute)
	if err := tx.Do(ctx, func(txCtx context.Context) error {
		if _, err := repo.SavePosition(txCtx, newPoint); err != nil {
			return err
		}
		_, err := repo.SavePosition(txCtx, oldPoint)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Get(ctx, old.ID)
	if err != nil || updated.LastPositionAt == nil || !updated.LastPositionAt.Equal(newPoint.RecordedAt) {
		t.Fatalf("last position = %v, error = %v", updated.LastPositionAt, err)
	}
	positions, err := repo.ListPositions(ctx, old.ID)
	if err != nil || len(positions) != 2 || !positions[0].RecordedAt.Equal(oldPoint.RecordedAt) || !positions[1].RecordedAt.Equal(newPoint.RecordedAt) {
		t.Fatalf("positions=%v,error=%v", positions, err)
	}
	found, err := repo.StaleTrips(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || (found[0].ID != old.ID && found[1].ID != old.ID) || (found[0].ID != withoutPosition.ID && found[1].ID != withoutPosition.ID) {
		t.Fatalf("stale trips=%v", found)
	}
	page, cursor, err := repo.StaleTripsBatch(ctx, time.Minute, 1, nil)
	if err != nil || len(page) != 1 || cursor == nil {
		t.Fatalf("first stale batch=%v,cursor=%v,error=%v", page, cursor, err)
	}
	page, cursor, err = repo.StaleTripsBatch(ctx, time.Minute, 1, cursor)
	if err != nil || len(page) != 1 || cursor == nil {
		t.Fatalf("second stale batch=%v,cursor=%v,error=%v", page, cursor, err)
	}
	page, cursor, err = repo.StaleTripsBatch(ctx, time.Minute, 1, cursor)
	if err != nil || len(page) != 0 || cursor != nil {
		t.Fatalf("third stale batch=%v,cursor=%v,error=%v", page, cursor, err)
	}
}

func TestPositionRollback(t *testing.T) {
	ctx, _, repo, _, tx := setup(t)
	fresh := newTrip()
	if err := repo.Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback")
	freshPoint := model.PositionInput{TripID: fresh.ID, Coordinates: model.Coordinates{Latitude: 59.8, Longitude: 30.4}, RecordedAt: fresh.StartedAt}
	if err := tx.Do(ctx, func(txCtx context.Context) error {
		_, err := repo.SavePosition(txCtx, freshPoint)
		if err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rollback error=%v", err)
	}
	positions, err := repo.ListPositions(ctx, fresh.ID)
	if err != nil || len(positions) != 0 {
		t.Fatalf("rollback positions=%v,error=%v", positions, err)
	}
	rolledBack, err := repo.Get(ctx, fresh.ID)
	if err != nil || rolledBack.LastPositionAt != nil {
		t.Fatalf("rollback last_position_at=%v, error=%v", rolledBack.LastPositionAt, err)
	}
}

func TestStaleCursorWithEqualTimestamps(t *testing.T) {
	ctx, _, repo, _, _ := setup(t)
	at := time.Now().Add(-time.Hour)
	expected := make(map[uuid.UUID]struct{})
	for range 5 {
		value := newTrip()
		value.StartedAt = at
		if err := repo.Create(ctx, value); err != nil {
			t.Fatal(err)
		}
		expected[value.ID] = struct{}{}
	}
	var cursor *model.StaleCursor
	for _, size := range []int{2, 2, 1, 0} {
		page, next, err := repo.StaleTripsBatch(ctx, time.Minute, 2, cursor)
		if err != nil || len(page) != size {
			t.Fatalf("page size=%d, expected=%d, error=%v", len(page), size, err)
		}
		for _, value := range page {
			if _, ok := expected[value.ID]; !ok {
				t.Fatalf("unexpected or repeated trip %s", value.ID)
			}
			delete(expected, value.ID)
		}
		cursor = next
	}
}

func newTrip() model.Trip {
	return model.Trip{ID: uuid.New(), UserID: uuid.New(), DriverID: uuid.New(), Status: domain.Active, Price: 1450, StartPoint: model.Coordinates{Latitude: 59.9, Longitude: 30.3}, EndPoint: model.Coordinates{Latitude: 59.8, Longitude: 30.4}, StartedAt: time.Now().UTC().Truncate(time.Microsecond)}
}

func TestRepositoryLifecycle(t *testing.T) {
	ctx, pool, repo, history, tx := setup(t)
	value := newTrip()
	if err := tx.Do(ctx, func(ctx context.Context) error {
		if err := repo.Create(ctx, value); err != nil {
			return err
		}
		return history.Add(ctx, model.StatusChange{TripID: value.ID, ToStatus: domain.Active, ChangedAt: value.StartedAt})
	}); err != nil {
		t.Fatal(err)
	}
	found, err := repo.Get(ctx, value.ID)
	if err != nil || !reflect.DeepEqual(found, value) {
		t.Fatalf("Get=(%+v,%v),want %+v", found, err, value)
	}
	duplicate := newTrip()
	duplicate.DriverID = value.DriverID
	if err := repo.Create(ctx, duplicate); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("duplicate error=%v", err)
	}
	at := value.StartedAt.Add(time.Minute)
	if err := tx.Do(ctx, func(ctx context.Context) error {
		completed, finishErr := repo.Finish(ctx, value.ID, at)
		if finishErr != nil {
			return finishErr
		}
		if completed.Status != domain.Completed || completed.FinishedAt == nil || !completed.FinishedAt.Equal(at) {
			return fmt.Errorf("invalid completed trip: %+v", completed)
		}
		from := domain.Active
		return history.Add(ctx, model.StatusChange{TripID: value.ID, FromStatus: &from, ToStatus: domain.Completed, ChangedAt: at})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Finish(ctx, value.ID, at); !errors.Is(err, domain.ErrTripCompleted) {
		t.Fatalf("repeat finish=%v", err)
	}
	if _, err := repo.Get(ctx, uuid.New()); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("get missing=%v", err)
	}
	if _, err := repo.Finish(ctx, uuid.New(), at); !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("finish missing=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM trip_status_history WHERE trip_id=$1", value.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("history count=%d", count)
	}
	if err := repo.Create(ctx, duplicate); err != nil {
		t.Fatalf("driver must be free after finish: %v", err)
	}
}

const panicMode = "panic"

func TestTransactionRollback(t *testing.T) {
	for _, mode := range []string{"second insert fails", "after two inserts", panicMode, "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, pool, repo, history, tx := setup(t)
			value := newTrip()
			sentinel := errors.New("abort transaction")
			run := func() error {
				return tx.Do(ctx, func(txCtx context.Context) error {
					if err := repo.Create(txCtx, value); err != nil {
						return err
					}
					status := domain.Active
					if mode == "second insert fails" {
						status = "invalid"
					}
					if err := history.Add(txCtx, model.StatusChange{TripID: value.ID, ToStatus: status, ChangedAt: value.StartedAt}); err != nil {
						return err
					}
					if mode == panicMode {
						panic(sentinel)
					}
					if mode == "cancelled" {
						cancelCtx, cancel := context.WithCancel(txCtx)
						cancel()
						return tx.Do(cancelCtx, func(ctx context.Context) error { _, err := repo.Get(ctx, value.ID); return err })
					}
					return sentinel
				})
			}
			if mode == panicMode {
				func() {
					defer func() {
						if p := recover(); p != sentinel {
							t.Fatalf("panic=%v", p)
						}
					}()
					_ = run()
					t.Error("panic was swallowed")
				}()
			} else if err := run(); err == nil {
				t.Fatal("expected transaction error")
			}
			for _, table := range []string{"trips", "trip_status_history"} {
				var count int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("%s contains %d rows after rollback", table, count)
				}
			}
		})
	}
}

func TestNestedTransaction(t *testing.T) {
	ctx, _, repo, history, tx := setup(t)
	value := newTrip()
	err := tx.Do(ctx, func(ctx context.Context) error {
		if err := repo.Create(ctx, value); err != nil {
			return err
		}
		return tx.Do(ctx, func(ctx context.Context) error {
			return history.Add(ctx, model.StatusChange{TripID: value.ID, ToStatus: domain.Active, ChangedAt: value.StartedAt})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, value.ID); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentCreate(t *testing.T) {
	ctx, _, repo, _, _ := setup(t)
	first, second := newTrip(), newTrip()
	second.DriverID = first.DriverID
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, value := range []model.Trip{first, second} {
		wg.Go(func() { results <- repo.Create(ctx, value) })
	}
	wg.Wait()
	close(results)
	success, busy := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrDriverBusy):
			busy++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || busy != 1 {
		t.Fatalf("success=%d,busy=%d", success, busy)
	}
}

func TestConcurrentFinish(t *testing.T) {
	ctx, _, repo, _, _ := setup(t)
	value := newTrip()
	if err := repo.Create(ctx, value); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := repo.Finish(ctx, value.ID, value.StartedAt.Add(time.Minute)); results <- err })
	}
	wg.Wait()
	close(results)
	success, completed := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, domain.ErrTripCompleted):
			completed++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || completed != 1 {
		t.Fatalf("success=%d,completed=%d", success, completed)
	}
}
