package store

import "sync"

// InMemoryBackend stores values in process memory. It is used for tests and
// no-db mode. Each instance is independent (not shared across domains).
type InMemoryBackend struct {
	mu   sync.RWMutex
	data map[Namespace]map[string]string
}

// NewInMemoryBackend creates an in-memory backend with all namespaces.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		data: map[Namespace]map[string]string{
			NamespaceContext:   {},
			NamespaceKnowledge: {},
			NamespaceEphemeral: {},
		},
	}
}

// Get implements Backend.
func (b *InMemoryBackend) Get(ns Namespace, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.data[ns] == nil {
		return "", nil
	}
	return b.data[ns][key], nil
}

// Set implements Backend.
func (b *InMemoryBackend) Set(ns Namespace, key, value string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.data[ns] == nil {
		b.data[ns] = map[string]string{}
	}
	b.data[ns][key] = value
	return nil
}

// Delete implements Backend.
func (b *InMemoryBackend) Delete(ns Namespace, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.data[ns] != nil {
		delete(b.data[ns], key)
	}
	return nil
}

func validateKey(key string) error {
	if key == "" {
		return ErrInvalidKey
	}
	return nil
}
