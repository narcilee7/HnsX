// Package memory provides context storage backends.
package memory

// Backend is a memory storage backend.
type Backend interface {
	Get(key string) (string, error)
	Set(key string, value string) error
}

// InMemoryBackend stores context in memory.
type InMemoryBackend struct {
	data map[string]string
}

// NewInMemoryBackend creates an in-memory backend.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{data: make(map[string]string)}
}

func (b *InMemoryBackend) Get(key string) (string, error) {
	return b.data[key], nil
}

func (b *InMemoryBackend) Set(key string, value string) error {
	b.data[key] = value
	return nil
}
