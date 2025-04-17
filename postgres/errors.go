package postgres

import (
	pgx "github.com/jackc/pgx/v5"
)

var (
	ErrNoRows      = pgx.ErrNoRows
	ErrTooManyRows = pgx.ErrTooManyRows
)
