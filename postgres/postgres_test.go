package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/pashagolub/pgxmock/v4"

	"git.tatikoma.dev/corpix/atlas/errors"
)

func TestClientTxRollback(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	ctx := context.Background()

	t.Run("Rollback on function error", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(err, "Failed to create pgxmock pool")
		defer mockPool.Close()

		dataToInsert := "data-should-not-be-committed-error"
		expectedErr := "simulated error"

		mockPool.ExpectBegin()
		mockPool.ExpectExec("INSERT INTO items").
			WithArgs(dataToInsert).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mockPool.ExpectRollback()

		_, txErr := WithTxContext(ctx, mockPool, func(tx Tx) (any, error) {
			_, insertErr := tx.Exec(ctx, `INSERT INTO items (value) VALUES ($1)`, dataToInsert)
			require.NoError(insertErr, "Insert should succeed within the mock transaction")
			return nil, errors.New(expectedErr)
		})

		assert.ErrorContains(txErr, expectedErr, "WithTxContext should return the error from the function")

		err = mockPool.ExpectationsWereMet()
		assert.NoError(err, "There were unfulfilled expectations after rollback due to error")
	})

	t.Run("Rollback on function panic", func(t *testing.T) {
		mockPool, err := pgxmock.NewPool()
		require.NoError(err, "Failed to create pgxmock pool")
		defer mockPool.Close()

		dataToInsert := "data-should-not-be-committed-panic"
		panicMessage := "simulated panic"

		mockPool.ExpectBegin()
		mockPool.ExpectExec("INSERT INTO items").
			WithArgs(dataToInsert).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mockPool.ExpectRollback()

		assert.Panics(func() {
			_, _ = WithTxContext(ctx, mockPool, func(tx Tx) (any, error) {
				_, insertErr := tx.Exec(ctx, `INSERT INTO items (value) VALUES ($1)`, dataToInsert)
				require.NoError(insertErr, "Insert should succeed within the mock transaction")
				panic(panicMessage)
			})
		}, "WithTxContext should re-panic with the original panic message")

		err = mockPool.ExpectationsWereMet()
		assert.NoError(err, "There were unfulfilled expectations after rollback due to panic")
	})
}
