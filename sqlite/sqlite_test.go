package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewClient(dbPath, 5*time.Second)
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

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tx_test.db")

	db, err := NewClient(dbPath, 5*time.Second)
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
