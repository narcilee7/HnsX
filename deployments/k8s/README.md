# HnsX Kubernetes Deployment

This directory contains Kubernetes manifests for running HnsX components.

## Components

- `worker-deployment.yaml` — Python Runtime Worker pool.

The Control Plane (`hnsx-server`) is expected to be deployed separately, either
as a Deployment/Service in the same cluster or as an external endpoint. The
worker Deployment connects to it via `HNSX_CONTROL_PLANE_ADDR`.

## Worker auto-discovery

Workers self-register with the Control Plane through the gRPC `WorkerService`.
On startup, the Python worker process builds a `WorkerInfo` message that should
include Kubernetes metadata as labels:

| Label | Source |
|---|---|
| `k8s.pod` | `POD_NAME` |
| `k8s.namespace` | `POD_NAMESPACE` |
| `k8s.node` | `NODE_NAME` |
| `k8s.pod_ip` | `POD_IP` |

The Go Control Plane reads these labels from `WorkerInfo.Labels` for ops
debugging and future affinity scheduling. No central registry of workers is
required: workers appear in `ListRuntimes` as soon as they register and
heartbeat.

## Deploy

```bash
kubectl apply -f deployments/k8s/namespace.yaml
kubectl apply -f deployments/k8s/worker-deployment.yaml
```

Update `HNSX_CONTROL_PLANE_ADDR` in `worker-deployment.yaml` to point at your
Control Plane gRPC endpoint.

## Graceful shutdown

When a worker pod is terminated:

1. Kubernetes sends `SIGTERM`.
2. The pre-stop hook sleeps 10s to give the Control Plane time to detect the
   closed StreamChannel and requeue any in-flight sessions.
3. The Go Control Plane's `SchedulerService.RequeueSessions` puts unacknowledged
   sessions back onto the shared Redis queue so another worker can pick them up.
4. If the worker does not exit within `terminationGracePeriodSeconds`, it is
   killed with `SIGKILL`.

For this to be safe, sessions should be idempotent or the worker should report
terminal status before exiting.
