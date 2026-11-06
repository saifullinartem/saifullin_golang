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
	if err != nil || len(files) != 2 {
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
