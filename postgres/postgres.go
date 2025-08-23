package postgres

import (
	"context"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type Tx = pgx.Tx

// Pool defines the interface required by WithTxContext for a database connection pool.
// This allows for mocking in tests.
type Pool interface {
	Ping(ctx context.Context) error
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Stat() *pgxpool.Stat
	Close()
}

func NewClient(dsn string, timeout time.Duration) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to database")
	}
	err = pool.Ping(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to ping database")
	}
	return pool, nil
}

// WithTxContext executes a function within a database transaction.
// It automatically handles transaction begin, commit, and rollback based on the function's return.
// It also handles panics, ensuring a rollback occurs.
func WithTxContext[T any](ctx context.Context, dbc Pool, fn func(Tx) (T, error)) (T, error) {
	var result T
	tx, err := dbc.Begin(ctx)
	if err != nil {
		return result, errors.Wrap(err, errors.ErrBeginTx)
	}

	defer func() {
		// closure is required to capture err value after execution of fn
		if panicErr := recover(); panicErr != nil {
			panicStr := fmt.Errorf("%v", panicErr)
			_ = EndTxContext(ctx, tx, panicStr)
			panic(panicStr)
		} else {
			_ = EndTxContext(ctx, tx, err)
		}
	}()

	result, err = fn(tx)
	// pay attention, err is used inside defer to rollback tx
	// thats why can't just return fn results
	return result, err
}

func WithTx[T any](dbc Pool, fn func(Tx) (T, error)) (T, error) {
	return WithTxContext(context.Background(), dbc, fn)
}

func BeginTx(dbc Pool) (Tx, error) {
	return BeginTxContext(context.Background(), dbc)
}

func EndTx(tx Tx, err error) error {
	return EndTxContext(context.Background(), tx, err)
}

func BeginTxContext(ctx context.Context, dbc Pool) (Tx, error) {
	tx, err := dbc.Begin(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrBeginTx)
	}
	return tx, nil
}

func EndTxContext(ctx context.Context, tx Tx, err error) error {
	var recoverErr any
	if err != nil {
		goto fail
	}
	recoverErr = recover()
	if recoverErr != nil {
		switch v := recoverErr.(type) {
		case error:
			err = v
		default:
			err = errors.Errorf("%v", v)
		}
		goto fail
	}
	errors.Log(tx.Commit(ctx), errors.ErrCommitTx)

	return nil
fail:
	errors.Log(tx.Rollback(ctx), errors.ErrRollbackTx)

	return err
}
