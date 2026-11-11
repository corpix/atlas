package maps

import (
	"maps"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcurrent(t *testing.T) {
	var m Concurrent[string, int]

	_, ok := m.Load("a")
	assert.False(t, ok)

	m.Store("a", 1)
	v, ok := m.Load("a")
	assert.True(t, ok)
	assert.Equal(t, 1, v)

	v, loaded := m.LoadOrStore("a", 2)
	assert.True(t, loaded)
	assert.Equal(t, 1, v)

	v, loaded = m.LoadOrStore("b", 2)
	assert.False(t, loaded)
	assert.Equal(t, 2, v)

	assert.Equal(t, map[string]int{"a": 1, "b": 2}, maps.Collect(m.All()))

	v, loaded = m.LoadAndDel("a")
	assert.True(t, loaded)
	assert.Equal(t, 1, v)
	_, loaded = m.LoadAndDel("a")
	assert.False(t, loaded)

	m.Del("b")
	assert.Empty(t, maps.Collect(m.All()))
}

func TestConcurrentNilInterfaceValue(t *testing.T) {
	var m Concurrent[string, error]
	m.Store("a", nil)

	v, ok := m.Load("a")
	assert.True(t, ok)
	assert.NoError(t, v)
}

func TestConcurrentLoadAndDelClaimsOnce(t *testing.T) {
	var (
		m      Concurrent[int, int]
		wg     sync.WaitGroup
		mu     sync.Mutex
		claims int
	)
	m.Store(1, 1)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, loaded := m.LoadAndDel(1)
			if loaded {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, claims)
}
