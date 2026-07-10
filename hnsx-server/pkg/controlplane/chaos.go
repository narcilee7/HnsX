package controlplane

import (
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/worker"
)

// ChaosInjector provides fault-injection primitives for chaos testing the
// Control Plane / Worker scheduler. It is intentionally separate from the
// production scheduler surface so it can be compiled out or disabled in
// release builds.
type ChaosInjector struct {
	Registry *worker.Registry
	Sched    *SchedulerServiceServer
}

// EvictWorker removes a worker from the registry and requeues any in-flight
// sessions it was responsible for. Simulates a worker crash or forced
// termination without graceful shutdown.
func (i *ChaosInjector) EvictWorker(workerID string) bool {
	if i.Registry == nil {
		return false
	}
	if !i.Registry.Evict(workerID) {
		return false
	}
	if i.Sched != nil {
		i.Sched.RequeueSessions(workerID)
	}
	return true
}

// PartitionPull makes the scheduler silently drop PullSession requests from
// workerID for duration d. This simulates a network partition where the
// worker can reach the server but cannot pull new work.
func (i *ChaosInjector) PartitionPull(workerID string, d time.Duration) {
	if i.Sched == nil {
		return
	}
	i.Sched.PartitionPull(workerID, d)
}
