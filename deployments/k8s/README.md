# HnsX Kubernetes Deployment

This directory contains Kubernetes manifests and deployment guidance for running
**HnsX Control Plane** (`hnsx-server`) and **Python Runtime Workers**
(`hnsx-worker`) on a Kubernetes cluster.

---

## Architecture

```text
                    +-------------------+
  Console / CLI --> | hnsx-server (Go)  | <-- long-poll --+   +------------------+
                    |  - REST API       |                 |   | hnsx-worker pods |
                    |  - gRPC control   |                 +-> |  - pull sessions |
                    |  - SSE events     |                 |   |  - run agents    |
                    +---------+---------+                 |   +------------------+
                              |                           |
                              v                           v
                         [Postgres]                  [Redis Queue]
```

- **Control Plane** is stateless and can scale horizontally. It owns the session
  state machine, worker registry, policy decisions, and audit log.
- **Workers** are stateless runtime pods. They self-register on startup, pull
  sessions from the shared Redis queue, and run agents inside isolated child
  processes.
- **Redis** provides the persistent session queue so multiple Control Plane
  instances can share the same work queue without duplicate delivery.
- **Postgres** stores domain metadata, session state, traces, audit logs, and
  worker registrations.

---

## Prerequisites

- A Kubernetes cluster and `kubectl` configured to talk to it.
- Container images for `hnsx-server` and `hnsx-worker` available to the cluster.
  Use `latest` for local development; pin a version tag for production.
- Postgres and Redis reachable from the cluster. For local experimentation you
  can deploy them inside the cluster (see the example below).

## Build images

The repository currently does not ship a container build target. Build images
locally or in your CI pipeline, for example:

```bash
# Control Plane
docker build -t hnsx-server:latest -f hnsx-server/Dockerfile hnsx-server

# Python Worker
docker build -t hnsx-worker:latest -f hnsx-worker/Dockerfile hnsx-worker
```

For local clusters (e.g. `kind`, `k3d`, `minikube`) load the images after
building:

```bash
kind load docker-image hnsx-server:latest hnsx-worker:latest
```

---

## Quick start

### 1. Create the namespace

```bash
kubectl apply -f deployments/k8s/namespace.yaml
```

### 2. Deploy dependencies

If you already have Postgres and Redis, skip this step and create a `Secret`
(see below) pointing at them.

For a quick all-in-one local stack you can use Helm:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install hnsx-postgres bitnami/postgresql \
  --namespace hnsx \
  --set auth.postgresPassword=change-me
helm install hnsx-redis bitnami/redis \
  --namespace hnsx \
  --set auth.enabled=false
```

### 3. Deploy the Control Plane

Create a Secret with the runtime configuration:

```bash
kubectl create secret generic hnsx-server-config \
  --namespace hnsx \
  --from-literal=DATABASE_URL='postgres://postgres:change-me@hnsx-postgres-postgresql:5432/hnsx?sslmode=disable' \
  --from-literal=REDIS_URL='redis://hnsx-redis-master:6379/0'
```

Then apply an example Control Plane manifest:

```yaml
# hnsx-server-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hnsx-server
  namespace: hnsx
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: hnsx-server
  template:
    metadata:
      labels:
        app.kubernetes.io/name: hnsx-server
        app.kubernetes.io/component: control-plane
    spec:
      containers:
        - name: server
          image: hnsx-server:latest
          imagePullPolicy: IfNotPresent
          ports:
            - name: grpc
              containerPort: 50061
            - name: http
              containerPort: 8080
          envFrom:
            - secretRef:
                name: hnsx-server-config
          resources:
            requests:
              cpu: 250m
              memory: 256Mi
            limits:
              cpu: 2000m
              memory: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: hnsx-server
  namespace: hnsx
spec:
  selector:
    app.kubernetes.io/name: hnsx-server
  ports:
    - name: grpc
      port: 50061
      targetPort: 50061
    - name: http
      port: 8080
      targetPort: 8080
```

```bash
kubectl apply -f hnsx-server-deployment.yaml
```

### 4. Deploy the Worker pool

Edit `worker-deployment.yaml` and point `HNSX_CONTROL_PLANE_ADDR` at the Control
Plane gRPC service created above. The default value assumes the Control Plane is
running in the same namespace:

```yaml
env:
  - name: HNSX_CONTROL_PLANE_ADDR
    value: "hnsx-server.hnsx:50061"
```

Then apply the manifest:

```bash
kubectl apply -f deployments/k8s/worker-deployment.yaml
```

### 5. Verify

```bash
# Control Plane pods
kubectl get pods -n hnsx -l app.kubernetes.io/name=hnsx-server

# Worker pods
kubectl get pods -n hnsx -l app.kubernetes.io/name=hnsx-worker

# Registered workers (via gRPC reflection or hnsx CLI)
kubectl port-forward -n hnsx svc/hnsx-server 50061:50061
./bin/hnsx runtimes list
```

---

## Manifests in this directory

| File | Purpose |
|---|---|
| `namespace.yaml` | Creates the `hnsx` namespace. |
| `worker-deployment.yaml` | Python Worker pool. Self-registers with the Control Plane and pulls sessions via long-poll. |

The Control Plane manifest is intentionally kept as an inline example in this
README because image registries, resource limits, and ingress/SSL settings vary
between environments. Copy the example above and adjust it for your cluster.

---

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

---

## Scaling

Both components can be scaled independently:

```bash
# Scale the Control Plane
kubectl scale deployment hnsx-server -n hnsx --replicas=3

# Scale the Worker pool
kubectl scale deployment hnsx-worker -n hnsx --replicas=5
```

For production, configure a `HorizontalPodAutoscaler` based on CPU/memory or a
custom metric such as session queue depth.

---

## Graceful shutdown

### Worker pod termination

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

### Control Plane rolling update

Control Plane pods are stateless. During a rolling update, in-flight HTTP/gRPC
requests are drained via Kubernetes `terminationGracePeriodSeconds`. Redis and
Postgres keep the shared state, so new pods can take over immediately.

---

## Configuration reference

### Control Plane environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string. |
| `REDIS_URL` | yes | Redis connection string for the session queue. |
| `HNSX_GRPC_ADDR` | no | gRPC listen address. Default `:50061`. |
| `HNSX_HTTP_ADDR` | no | HTTP/REST listen address. Default `:8080`. |

### Worker environment variables

| Variable | Required | Description |
|---|---|---|
| `HNSX_CONTROL_PLANE_ADDR` | yes | gRPC endpoint of the Control Plane. |
| `HNSX_WORKER_REGION` | no | Logical region label, e.g. `k8s`. |
| `HNSX_WORKER_LABELS` | no | Comma-separated `key=value` capability labels. |
| `HNSX_WORKER_TENANT_ID` | no | Default tenant ID when running in single-tenant mode. |

---

## Troubleshooting

### Workers cannot reach the Control Plane

- Check DNS: from a worker pod, run `nslookup hnsx-server.hnsx`.
- Verify `HNSX_CONTROL_PLANE_ADDR` matches the gRPC port (`50061`), not the HTTP
  port (`8080`).
- Ensure the `hnsx-server` Service selector labels match the Control Plane pod
  labels.

### Sessions are not being picked up

- Confirm Redis is reachable from the Control Plane (`REDIS_URL`).
- Check that workers are registered: `runtimes list` should show healthy workers.
- Verify worker labels satisfy the capability constraints of the queued session.

### Duplicate session delivery

- Ensure all Control Plane replicas use the **same** `REDIS_URL` and Postgres.
- Do not run multiple isolated Redis instances behind different Control Plane
  pods.

---

## Production checklist

- [ ] Pin image tags instead of `latest`.
- [ ] Use an external managed Postgres and Redis with TLS.
- [ ] Store credentials in a secrets manager (Sealed Secrets, External Secrets,
      Vault, etc.).
- [ ] Configure pod disruption budgets for the Control Plane.
- [ ] Add ingress/TLS for the HTTP API and gRPC gateway.
- [ ] Set up liveness/readiness probes on the Control Plane.
- [ ] Configure `HorizontalPodAutoscaler` for workers based on queue depth.
