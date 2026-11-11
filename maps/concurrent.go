package maps

import (
	"iter"
	"sync"
)

// Concurrent is a typed sync.Map, safe for concurrent use
type Concurrent[K comparable, V any] struct {
	m sync.Map
}

func (m *Concurrent[K, V]) Load(key K) (V, bool) {
	v, ok := m.m.Load(key)
	return m.value(v), ok
}

func (m *Concurrent[K, V]) Store(key K, value V) {
	m.m.Store(key, value)
}

// LoadOrStore returns the stored value if present, otherwise it stores and returns value
func (m *Concurrent[K, V]) LoadOrStore(key K, value V) (V, bool) {
	v, loaded := m.m.LoadOrStore(key, value)
	return m.value(v), loaded
}

// LoadAndDel deletes the value for a key and returns it, only one concurrent caller gets loaded true
func (m *Concurrent[K, V]) LoadAndDel(key K) (V, bool) {
	v, loaded := m.m.LoadAndDelete(key)
	return m.value(v), loaded
}

func (m *Concurrent[K, V]) Del(key K) {
	m.m.Delete(key)
}

func (m *Concurrent[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		m.m.Range(func(k, v any) bool {
			return yield(k.(K), m.value(v))
		})
	}
}

func (m *Concurrent[K, V]) value(v any) V {
	out, _ := v.(V)
	return out
}

func NewConcurrent[K comparable, V any]() *Concurrent[K, V] {
	return &Concurrent[K, V]{}
}
