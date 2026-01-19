package app

import (
	"fmt"
)

type (
	Meta map[string]any
)

var MetaRegistry = Meta{}

type ErrMetaAlreadyRegistered struct {
	Key string
}

func (e ErrMetaAlreadyRegistered) Error() string {
	return fmt.Sprintf("meta key already registered: %q", e.Key)
}

type ErrMetaNotRegistered struct {
	Key string
}

func (e ErrMetaNotRegistered) Error() string {
	return fmt.Sprintf("meta key not registered: %q", e.Key)
}

// Register stores a key/value pair in the registry.
// Returns an error if the key already registered.
func (m Meta) Register(key string, value any) error {
	if _, ok := m[key]; ok {
		return ErrMetaAlreadyRegistered{Key: key}
	}
	m[key] = value
	return nil
}

// Set assign a new value for key in the registry.
// Returns an error if the key is not registered.
func (m Meta) Set(key string, value any) error {
	_, ok := m[key]
	if !ok {
		return ErrMetaNotRegistered{Key: key}
	}
	m[key] = value
	return nil
}

// Lookup returns the value associated with the key.
// Returns an error if the key is not registered.
func (m Meta) Lookup(key string) (any, error) {
	v, ok := m[key]
	if !ok {
		return v, ErrMetaNotRegistered{Key: key}
	}
	return v, nil
}

func (m Meta) MustRegister(key string, value any) {
	if err := m.Register(key, value); err != nil {
		panic(err)
	}
}

func (m Meta) MustSet(key string, value any) {
	if err := m.Set(key, value); err != nil {
		panic(err)
	}
}

func (m Meta) MustLookup(key string) any {
	v, err := m.Lookup(key)
	if err != nil {
		panic(err)
	}
	return v
}

// Iter returns an idiomatic iterator over all key/value pairs.
func (m Meta) Iter() func(yield func(key string, value any) bool) {
	return func(yield func(key string, value any) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}

// MetaRegister stores a key/value pair in MetaRegistry.
// Returns an error if the key already registered.
func MetaRegister(key string, value any) error {
	return MetaRegistry.Register(key, value)
}

// MetaSet assigns a new value for key in MetaRegistry.
// Returns an error if the key is not registered.
func MetaSet(key string, value any) error {
	return MetaRegistry.Set(key, value)
}

// MetaLookup returns the value associated with the key.
// Returns an error if the key is not registered.
func MetaLookup(key string) (any, error) {
	return MetaRegistry.Lookup(key)
}

// MetaMustRegister panics if the key already registered.
func MetaMustRegister(key string, value any) {
	MetaRegistry.MustRegister(key, value)
}

// MetaMustSet panics if the key is not registered.
func MetaMustSet(key string, value any) {
	MetaRegistry.MustSet(key, value)
}

// MetaMustLookup panics if the key is not registered.
func MetaMustLookup(key string) any {
	return MetaRegistry.MustLookup(key)
}

// MetaIter returns an idiomatic iterator over all key/value pairs
// stored in MetaRegistry.
func MetaIter() func(yield func(key string, value any) bool) {
	return MetaRegistry.Iter()
}
