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

func TestSortedDelWhileIterating(t *testing.T) {
	var m Sorted[string, int]
	for n, key := range []string{"a", "b", "c", "d"} {
		m.Set(key, n)
	}

	var seen []string
	for key := range m.All() {
		seen = append(seen, key)
		m.Del(key)
		if key == "b" {
			m.Del("c")
		}
	}

	assert.Equal(t, []string{"a", "b", "d"}, seen)
	assert.Equal(t, 0, m.Len())
}

func TestSortedSetWhileIterating(t *testing.T) {
	var m Sorted[string, int]
	m.Set("b", 0)
	m.Set("d", 0)

	var seen []string
	for key := range m.Keys() {
		seen = append(seen, key)
		if key == "b" {
			m.Set("a", 0)
			m.Set("c", 0)
		}
	}

	assert.Equal(t, []string{"b", "c", "d"}, seen)
}
