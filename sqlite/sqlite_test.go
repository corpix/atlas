package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	db, err := NewClient(":memory:", 5*time.Second)
	require.NoError(err)
	require.NotNil(db)
	defer func() {
		closeErr := db.Close()
		require.NoError(closeErr)
	}()

	_, err = db.ExecContext(ctx, `CREATE TABLE test_items (id INTEGER PRIMARY KEY AUTOINCREMENT, data TEXT NOT NULL)`)
	require.NoError(err)

	testData := "some important data"
	res, err := db.ExecContext(ctx, `INSERT INTO test_items (data) VALUES (?)`, testData)
	require.NoError(err)

	lastID, err := res.LastInsertId()
	require.NoError(err)
	require.True(lastID > 0)

	var retrievedData string
	err = db.QueryRowContext(ctx, `SELECT data FROM test_items WHERE id = ?`, lastID).Scan(&retrievedData)
	require.NoError(err)
	require.Equal(testData, retrievedData)
}

func TestClientTx(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	db, err := NewClient(":memory:", 5*time.Second)
	require.NoError(err)
	require.NotNil(db)
	defer func() {
		closeErr := db.Close()
		assert.NoError(t, closeErr)
	}()

	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tx_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`)
	require.NoError(err)

	simpleData := "simple tx path data"
	_, err = WithTxContext(ctx, db, func(tx *Tx) (any, error) {
		_, insertErr := tx.ExecContext(ctx, `INSERT INTO tx_items (name) VALUES (?)`, simpleData)
		return nil, insertErr
	})
	require.NoError(err)

	var retrievedName string
	err = db.QueryRowContext(ctx, `SELECT name FROM tx_items WHERE name = ?`, simpleData).Scan(&retrievedName)
	require.NoError(err)
	require.Equal(simpleData, retrievedName)
}

func TestClientTxRollback(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	ctx := context.Background()

	db, err := NewClient(":memory:", 5*time.Second)
	require.NoError(err)
	require.NotNil(db)
	defer func() {
		closeErr := db.Close()
		assert.NoError(closeErr)
	}()

	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS rollback_items (id INTEGER PRIMARY KEY AUTOINCREMENT, value TEXT NOT NULL)`)
	require.NoError(err)

	t.Run("Rollback on function error", func(t *testing.T) {
		dataToInsert := "data-should-not-be-committed-error"
		expectedErr := "simulated error"

		_, txErr := WithTxContext(ctx, db, func(tx *Tx) (any, error) {
			_, insertErr := tx.ExecContext(ctx, `INSERT INTO rollback_items (value) VALUES (?)`, dataToInsert)
			require.NoError(insertErr, "Insert should succeed within the transaction")
			return nil, fmt.Errorf(expectedErr)
		})

		assert.ErrorContains(txErr, expectedErr, "WithTxContext should return the error from the function")

		var count int
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rollback_items WHERE value = ?`, dataToInsert).Scan(&count)
		require.NoError(err, "Failed to query count after rollback attempt")
		assert.Zero(count, "Data should not be committed after function returns an error")
	})

	t.Run("Rollback on function panic", func(t *testing.T) {
		dataToInsert := "data-should-not-be-committed-panic"
		panicMessage := "simulated panic"

		assert.Panics(func() {
			_, _ = WithTxContext(ctx, db, func(tx *Tx) (any, error) {
				_, insertErr := tx.ExecContext(ctx, `INSERT INTO rollback_items (value) VALUES (?)`, dataToInsert)
				require.NoError(insertErr, "Insert should succeed within the transaction")
				panic(panicMessage)
			})
		}, "WithTxContext should re-panic with the original panic message")

		var count int
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rollback_items WHERE value = ?`, dataToInsert).Scan(&count)
		require.NoError(err, "Failed to query count after panic rollback attempt")
		assert.Zero(count, "Data should not be committed after function panics")
	})
}
