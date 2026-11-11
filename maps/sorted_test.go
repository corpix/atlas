package maps

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSorted(t *testing.T) {
	m := NewSorted[string, int]()
	m.Set("c", 3)
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 10)

	assert.Equal(t, 3, m.Len())
	assert.Equal(t, []string{"a", "b", "c"}, slices.Collect(m.Keys()))
	assert.Equal(t, []int{10, 2, 3}, slices.Collect(m.Values()))

	v, ok := m.Get("b")
	assert.True(t, ok)
	assert.Equal(t, 2, v)

	_, ok = m.Get("z")
	assert.False(t, ok)

	assert.True(t, m.Del("b"))
	assert.False(t, m.Del("b"))
	assert.False(t, m.Has("b"))
	assert.Equal(t, []string{"a", "c"}, slices.Collect(m.Keys()))

	var keys []string
	for key := range m.All() {
		keys = append(keys, key)
		break
	}
	assert.Equal(t, []string{"a"}, keys)
}

func TestSortedZeroValue(t *testing.T) {
	var m Sorted[string, int]
	assert.False(t, m.Has("a"))
	assert.False(t, m.Del("a"))

	m.Set("b", 2)
	m.Set("a", 1)
	assert.Equal(t, []string{"a", "b"}, slices.Collect(m.Keys()))
}
