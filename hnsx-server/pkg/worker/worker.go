// Package worker re-exports the internal worker scheduler types for backward
// compatibility during the DDD migration. New code should prefer
// internal/worker/service or internal/worker directly.
package worker

import (
	"github.com/redis/go-redis/v9"

	"github.com/hnsx-io/hnsx/server/internal/worker"
)

// Registry tracks live Python Runtime Workers.
type Registry = worker.Registry

// SessionQueue is the scheduler queue used by workers to pull sessions.
type SessionQueue = worker.SessionQueue

// SessionRequest is a pending session available for a worker to pull.
type SessionRequest = worker.SessionRequest

// Snapshot is a read-only view of a worker's runtime state.
type Snapshot = worker.Snapshot

// ErrUnknownWorker is returned when a worker_id is not registered.
var ErrUnknownWorker = worker.ErrUnknownWorker

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry { return worker.NewRegistry() }

// NewSessionQueue constructs an empty in-memory SessionQueue.
func NewSessionQueue() SessionQueue { return worker.NewSessionQueue() }

// NewMemorySessionQueue is an explicit alias of NewSessionQueue. Use it when
// the caller wants to document that an in-memory queue is intentional.
func NewMemorySessionQueue() SessionQueue { return worker.NewSessionQueue() }

// NewRedisSessionQueue constructs a queue backed by the supplied Redis client.
func NewRedisSessionQueue(rdb *redis.Client, prefix string) SessionQueue {
	return worker.NewRedisSessionQueue(rdb, prefix)
}
