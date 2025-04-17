package postgres

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type (
	Tx   = pgx.Tx
	Pool = pgxpool.Pool
)

func NewClient(dsn string, timeout time.Duration) (*Pool, error) {
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

func WithTxContext[T any](ctx context.Context, dbc *Pool, fn func(Tx) (T, error)) (T, error) {
	var result  T
	tx, err := dbc.Begin(ctx)
	if err != nil {
		return result, errors.Wrap(err, errors.ErrBeginTx)
	}

	defer func() {
		// closure is required to capture err value after execution of fn
		_ = EndTxContext(ctx, tx, err)
	}()

	result, err = fn(tx)
	// pay attention, err is used inside defer to rollback tx
	// thats why can't just return fn results
	return result, err
}

func WithTx[T any](dbc *Pool, fn func(Tx) (T, error)) (T, error) {
	return WithTxContext(context.Background(), dbc, fn)
}

func BeginTx(dbc *Pool) (Tx, error) {
	return BeginTxContext(context.Background(), dbc)
}

func EndTx(tx Tx, err error) error {
	return EndTxContext(context.Background(), tx, err)
}

func BeginTxContext(ctx context.Context, dbc *Pool) (Tx, error) {
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
