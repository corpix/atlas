package postgres

import (
	"github.com/jackc/pgerrcode"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.tatikoma.dev/corpix/atlas/errors"
)

var (
	ErrNoRows      = pgx.ErrNoRows
	ErrTooManyRows = pgx.ErrTooManyRows
)

func ErrIsNoRows(err error) bool {
	return errors.Is(err, ErrNoRows)
}

func ErrIsConflict(err error) bool {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		if pgErr.Code == pgerrcode.UniqueViolation {
			return true
		}
	}
	return false
}
