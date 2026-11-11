package maps

import (
	"cmp"
	"iter"
	"slices"
)

// Sorted is a map that iterates in ascending key order
type Sorted[K cmp.Ordered, V any] struct {
	kv   map[K]V
	keys []K
}

func (m *Sorted[K, V]) Get(key K) (V, bool) {
	v, ok := m.kv[key]
	return v, ok
}

func (m *Sorted[K, V]) Has(key K) bool {
	_, ok := m.kv[key]
	return ok
}

func (m *Sorted[K, V]) Set(key K, value V) {
	_, ok := m.kv[key]
	if !ok {
		n, _ := slices.BinarySearch(m.keys, key)
		m.keys = slices.Insert(m.keys, n, key)
	}
	m.kv[key] = value
}

func (m *Sorted[K, V]) Del(key K) bool {
	_, ok := m.kv[key]
	if !ok {
		return false
	}
	n, _ := slices.BinarySearch(m.keys, key)
	m.keys = slices.Delete(m.keys, n, n+1)
	delete(m.kv, key)
	return true
}

func (m *Sorted[K, V]) Len() int {
	return len(m.keys)
}

func (m *Sorted[K, V]) Keys() iter.Seq[K] {
	return slices.Values(m.keys)
}

func (m *Sorted[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, key := range m.keys {
			if !yield(m.kv[key]) {
				return
			}
		}
	}
}

func (m *Sorted[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, key := range m.keys {
			if !yield(key, m.kv[key]) {
				return
			}
		}
	}
}

func NewSorted[K cmp.Ordered, V any]() *Sorted[K, V] {
	return &Sorted[K, V]{kv: map[K]V{}}
}
