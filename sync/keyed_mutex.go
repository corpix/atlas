package sync

import (
	"context"
	"sync"
)

type void = struct{}

// KeyedMutex serializes work per key without holding a lock over unrelated
// keys, so a critical section may span a slow call.
type KeyedMutex[K comparable] struct {
	held map[K]chan void
	sync.Mutex
}

func (m *KeyedMutex[K]) Lock(ctx context.Context, key K) error {
	for {
		m.Mutex.Lock()
		released, ok := m.held[key]
		if !ok {
			m.held[key] = make(chan void)
			m.Mutex.Unlock()
			return nil
		}
		m.Mutex.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-released:
		}
	}
}

func (m *KeyedMutex[K]) Unlock(key K) {
	m.Mutex.Lock()
	released, ok := m.held[key]
	delete(m.held, key)
	m.Mutex.Unlock()
	if ok {
		close(released)
	}
}

func (m *KeyedMutex[K]) With(ctx context.Context, key K, fn func() error) error {
	err := m.Lock(ctx, key)
	if err != nil {
		return err
	}
	defer m.Unlock(key)
	return fn()
}

func NewKeyedMutex[K comparable]() *KeyedMutex[K] {
	return &KeyedMutex[K]{held: map[K]chan void{}}
}
