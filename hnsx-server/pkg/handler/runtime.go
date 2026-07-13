package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/internal/worker"
	workermodel "github.com/hnsx-io/hnsx/server/internal/worker/model"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListRuntimesInput struct {
	TenantID tenant.ID
	Limit    int
	Offset   int
}

type GetRuntimeInput struct {
	TenantID  tenant.ID
	RuntimeID string
}

type DiscoverRuntimesInput struct {
	TenantID     tenant.ID
	Capabilities []string
	Region       string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListRuntimesOutput struct {
	Runtimes viewmodel.RuntimeList
}

type GetRuntimeOutput struct {
	Runtime *viewmodel.RuntimeDetail
}

type DiscoverRuntimesOutput struct {
	Runtimes []viewmodel.RuntimeInfo
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListRuntimes returns every live runtime worker registered with the control
// plane. WorkerService is only wired when the gRPC control plane is enabled;
// when it is nil an empty list is returned so callers can render an honest
// empty state.
func (h *Handler) ListRuntimes(ctx context.Context, in ListRuntimesInput) (*ListRuntimesOutput, error) {
	defer h.hook(ctx, "runtime.list", zap.String("tenant_id", string(in.TenantID)))()

	out := make([]viewmodel.RuntimeListItem, 0)
	if h.App == nil || h.App.WorkerService == nil {
		return &ListRuntimesOutput{Runtimes: viewmodel.RuntimeList{
			Items:  out,
			Total:  0,
			Limit:  0,
			Offset: in.Offset,
		}}, nil
	}

	snaps := h.App.WorkerService.List()
	out = make([]viewmodel.RuntimeListItem, 0, len(snaps))
	for _, snap := range snaps {
		out = append(out, h.toRuntimeListItem(snap))
	}

	limit := in.Limit
	if limit <= 0 {
		limit = len(out)
	}
	return &ListRuntimesOutput{Runtimes: viewmodel.RuntimeList{
		Items:  out,
		Total:  len(out),
		Limit:  limit,
		Offset: in.Offset,
	}}, nil
}

// GetRuntime returns a single runtime worker detail.
func (h *Handler) GetRuntime(ctx context.Context, in GetRuntimeInput) (*GetRuntimeOutput, error) {
	defer h.hook(ctx, "runtime.get", zap.String("tenant_id", string(in.TenantID)), zap.String("runtime_id", in.RuntimeID))()

	if h.App == nil || h.App.WorkerService == nil {
		return nil, workermodel.ErrWorkerNotFound
	}
	snap, ok := h.App.WorkerService.Get(in.RuntimeID)
	if !ok {
		return nil, workermodel.ErrWorkerNotFound
	}
	return &GetRuntimeOutput{Runtime: h.toRuntimeDetail(snap)}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *Handler) toRuntimeListItem(snap worker.Snapshot) viewmodel.RuntimeListItem {
	item := viewmodel.RuntimeListItem{
		RuntimeID:       snap.WorkerID,
		Status:          runtimeStatus(snap),
		LastHeartbeatAt: snap.LastSeen,
		AgeSeconds:      snap.AgeSeconds,
		Healthy:         snap.Healthy,
	}
	if snap.Info == nil {
		return item
	}

	item.Version = snap.Info.Version
	item.Region = snap.Info.Region
	item.Hostname = snap.Info.Hostname
	item.Pid = snap.Info.Pid

	if snap.Info.Capacity != nil {
		if snap.Info.Capacity.MaxConcurrentSessions > 0 {
			item.Capacity = snap.Info.Capacity.MaxConcurrentSessions
		}
		if len(snap.Info.Capacity.Providers) > 0 {
			item.Capabilities = append([]string(nil), snap.Info.Capacity.Providers...)
		}
		if len(snap.Info.Capacity.Models) > 0 {
			item.Models = append([]string(nil), snap.Info.Capacity.Models...)
		}
		if len(snap.Info.Capacity.SandboxRuntimes) > 0 {
			item.SandboxRuntimes = append([]string(nil), snap.Info.Capacity.SandboxRuntimes...)
		}
	}

	if len(snap.Info.Labels) > 0 {
		item.Labels = make(map[string]string, len(snap.Info.Labels))
		for k, v := range snap.Info.Labels {
			item.Labels[k] = v
		}
	}

	return item
}

func (h *Handler) toRuntimeDetail(snap worker.Snapshot) *viewmodel.RuntimeDetail {
	item := h.toRuntimeListItem(snap)
	return &viewmodel.RuntimeDetail{
		RuntimeID:       item.RuntimeID,
		Status:          item.Status,
		LastHeartbeatAt: item.LastHeartbeatAt,
		AgeSeconds:      item.AgeSeconds,
		Healthy:         item.Healthy,
		Version:         item.Version,
		Region:          item.Region,
		Hostname:        item.Hostname,
		Pid:             item.Pid,
		Capacity:        item.Capacity,
		Capabilities:    item.Capabilities,
		Models:          item.Models,
		SandboxRuntimes: item.SandboxRuntimes,
		Labels:          item.Labels,
	}
}

// runtimeStatus maps the registry's liveness signal into the wire-level status
// vocabulary that the UI expects:
//   - healthy  : last heartbeat within the healthy threshold
//   - degraded : older but not yet evicted
//   - offline  : beyond the soft-eviction threshold (60s)
func runtimeStatus(snap worker.Snapshot) string {
	if !snap.LastSeen.IsZero() && snap.AgeSeconds > 60 {
		return "offline"
	}
	if snap.Healthy {
		return "healthy"
	}
	return "degraded"
}

// DiscoverRuntimes returns runtimes whose capabilities satisfy the requested
// set and whose region matches when one is supplied.
func (h *Handler) DiscoverRuntimes(ctx context.Context, in DiscoverRuntimesInput) (*DiscoverRuntimesOutput, error) {
	defer h.hook(ctx, "runtime.discover", zap.String("tenant_id", string(in.TenantID)))()

	out := make([]viewmodel.RuntimeInfo, 0)
	if h.App == nil || h.App.WorkerService == nil {
		return &DiscoverRuntimesOutput{Runtimes: out}, nil
	}

	required := in.Capabilities
	region := in.Region
	for _, w := range h.App.WorkerService.List() {
		info := w.Info
		if info == nil {
			continue
		}
		if region != "" && info.GetRegion() != region {
			continue
		}
		caps := runtimeCapabilities(info)
		if !hasAllCapabilities(caps, required) {
			continue
		}
		out = append(out, viewmodel.RuntimeInfo{
			RuntimeID:    w.WorkerID,
			Capabilities: caps,
			Region:       info.GetRegion(),
			Version:      info.GetVersion(),
		})
	}
	return &DiscoverRuntimesOutput{Runtimes: out}, nil
}

func runtimeCapabilities(info *pb.WorkerInfo) []string {
	if info == nil || info.Capacity == nil {
		return nil
	}
	c := info.Capacity
	var caps []string
	caps = append(caps, c.GetProviders()...)
	caps = append(caps, c.GetModels()...)
	caps = append(caps, c.GetSandboxRuntimes()...)
	return caps
}

func hasAllCapabilities(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, h := range have {
		set[h] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}

// ErrRuntimeNotFound is re-exported from the worker model so HTTP/gRPC can
// compare directly.
var ErrRuntimeNotFound = workermodel.ErrWorkerNotFound

// IsRuntimeNotFound reports whether err is a runtime-not-found error.
func IsRuntimeNotFound(err error) bool {
	return errors.Is(err, workermodel.ErrWorkerNotFound)
}
