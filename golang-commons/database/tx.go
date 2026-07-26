package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKey int

const txCtxKey ctxKey = 1

// DBTX is the common subset of *pgxpool.Pool and pgx.Tx used by repositories.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Conn returns the transaction bound to ctx, or pool when none is present.
func Conn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txCtxKey).(pgx.Tx); ok && tx != nil {
		return tx
	}
	return pool
}

// WithinTransaction runs fn inside a DB transaction bound to ctx.
func WithinTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txCtxKey).(pgx.Tx); ok {
		return fn(ctx)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(context.WithValue(ctx, txCtxKey, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Transactor wraps WithinTransaction for injection into services.
type Transactor struct {
	Pool *pgxpool.Pool
}

// WithinTransaction implements the service-level transaction port.
func (t *Transactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return WithinTransaction(ctx, t.Pool, fn)
}
