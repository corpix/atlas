package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

func ErrIsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
