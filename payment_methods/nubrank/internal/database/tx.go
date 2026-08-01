package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of *pgxpool.Pool's API that repositories use.
// pgx.Tx satisfies it too, so a repository written against Querier works
// unchanged whether it's talking to the pool directly or to an in-progress
// transaction.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// TxRunner runs a function against a single database transaction, so
// multiple repository calls made through the Querier it provides commit or
// roll back together — e.g. writing a payment and its outbox event
// atomically.
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(q Querier) error) error
}

type poolTxRunner struct {
	pool *pgxpool.Pool
}

func NewTxRunner(pool *pgxpool.Pool) TxRunner {
	return &poolTxRunner{pool: pool}
}

func (r *poolTxRunner) RunInTx(ctx context.Context, fn func(q Querier) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
