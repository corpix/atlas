package sqlite

import (
	"database/sql"

	sqlite "github.com/mattn/go-sqlite3"

	"git.tatikoma.dev/corpix/atlas/errors"
)

var (
	ErrNoRows = sql.ErrNoRows
)

func ErrIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func ErrIsConflict(err error) bool {
	var sqliteErr sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.ExtendedCode {
		case sqlite.ErrConstraintUnique,
			sqlite.ErrConstraintPrimaryKey,
			sqlite.ErrConstraintForeignKey,
			sqlite.ErrConstraintNotNull,
			sqlite.ErrConstraintCheck,
			sqlite.ErrConstraintTrigger,
			sqlite.ErrConstraintRowID:
			return true
		}
	}
	return false
}
