package sync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyedMutexSerializesOneKey(t *testing.T) {
	var (
		m       KeyedMutex[string]
		wg      sync.WaitGroup
		inside  atomic.Int32
		overlap atomic.Bool
		count   int
	)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.With(context.Background(), "a", func() error {
				if inside.Add(1) > 1 {
					overlap.Store(true)
				}
				count++
				inside.Add(-1)
				return nil
			})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.False(t, overlap.Load())
	assert.Equal(t, 32, count)
}

func TestKeyedMutexKeysAreIndependent(t *testing.T) {
	var m KeyedMutex[string]
	require.NoError(t, m.Lock(context.Background(), "a"))
	defer m.Unlock("a")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, m.Lock(ctx, "b"))
	m.Unlock("b")
}

func TestKeyedMutexLockReturnsOnCancel(t *testing.T) {
	m := NewKeyedMutex[string]()
	require.NoError(t, m.Lock(context.Background(), "a"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, m.Lock(ctx, "a"), context.Canceled)

	acquired := make(chan error)
	go func() { acquired <- m.Lock(context.Background(), "a") }()
	m.Unlock("a")
	require.NoError(t, <-acquired)
	m.Unlock("a")
}
