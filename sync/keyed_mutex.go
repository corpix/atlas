package sync

import (
	"context"

	"git.tatikoma.dev/corpix/atlas/maps"
)

type void = struct{}

// KeyedMutex serializes work per key without holding a lock over unrelated
// keys, so a critical section may span a slow call.
type KeyedMutex[K comparable] struct {
	held maps.Concurrent[K, chan void]
}

func (m *KeyedMutex[K]) Lock(ctx context.Context, key K) error {
	for {
		released, loaded := m.held.LoadOrStore(key, make(chan void))
		if !loaded {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-released:
		}
	}
}

func (m *KeyedMutex[K]) Unlock(key K) {
	released, ok := m.held.LoadAndDel(key)
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
	return &KeyedMutex[K]{}
}
