package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type (
	Tx = sql.Tx
	DB = sql.DB
)

var DefaultPragmas = []string{
	"journal_mode=WAL",
	"foreign_keys=1",
	"busy_timeout=5000",
	"synchronous=NORMAL",
}

func NewClient(dsn string, timeout time.Duration) (*DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite database: %s", dsn)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for _, pragma := range DefaultPragmas {
		_, err = db.ExecContext(ctx, fmt.Sprintf("PRAGMA %s;", pragma))
		if err != nil {
			_ = db.Close()
			return nil, errors.Wrapf(err, "failed to execute pragma: %s", pragma)
		}
	}

	err = db.PingContext(ctx)
	if err != nil {
		_ = db.Close()
		return nil, errors.Wrap(err, "failed to ping sqlite database")
	}

	return db, nil
}

func WithTxContext[T any](ctx context.Context, db *DB, fn func(tx *Tx) (T, error)) (T, error) {
	var result T
	tx, err := db.BeginTx(ctx, nil)
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

func WithTx[T any](db *DB, fn func(tx *Tx) (T, error)) (T, error) {
	return WithTxContext(context.Background(), db, fn)
}

func BeginTx(db *DB) (*Tx, error) {
	return BeginTxContext(context.Background(), db)
}

func EndTx(tx *Tx, err error) error {
	return EndTxContext(context.Background(), tx, err)
}

func BeginTxContext(ctx context.Context, db *DB) (*Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrBeginTx)
	}
	return tx, nil
}

func EndTxContext(ctx context.Context, tx *Tx, err error) error {
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			errors.Log(rbErr, errors.ErrRollbackTx)
		}
		return err
	}

	if cmtErr := tx.Commit(); cmtErr != nil {
		return errors.Wrap(cmtErr, errors.ErrCommitTx)
	}

	return nil
}
