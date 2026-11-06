package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type transactionKey struct{}

type transactionState struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

type TxManager struct {
	pool      *pgxpool.Pool
	timeout   time.Duration
	isolation pgx.TxIsoLevel
}

func NewTxManager(pool *pgxpool.Pool, timeout time.Duration) *TxManager {
	return &TxManager{pool: pool, timeout: timeout, isolation: pgx.ReadCommitted}
}

func (m *TxManager) Do(ctx context.Context, fn func(context.Context) error) (err error) {
	if state, ok := ctx.Value(transactionKey{}).(transactionState); ok && state.pool == m.pool {
		return fn(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: m.isolation})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
		defer cleanupCancel()
		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) && err != nil {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	txCtx := context.WithValue(ctx, transactionKey{}, transactionState{pool: m.pool, tx: tx})
	if err = fn(txCtx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func ExecutorFromContext(ctx context.Context, pool Executor) Executor {
	if state, ok := ctx.Value(transactionKey{}).(transactionState); ok && state.pool == pool {
		return state.tx
	}
	return pool
}
