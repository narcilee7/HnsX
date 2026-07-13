// Package controlplane is the gRPC/Connect control plane entrypoint.
//
// This file implements the five services defined in proto/hnsx/v1/control_plane.proto
// as Connect handlers. They are backed by the same application services that power
// the REST API, so the two transports stay in sync.
package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/obs"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"go.uber.org/zap"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1/v1connect"
)

// ConnectServer implements the 5 control_plane.proto services.
type ConnectServer struct {
	App *app.Application
	// Handlers is the shared business kernel used by both REST and Connect.
	Handlers *handler.Handler
	// Logger is the request-scoped structured logger used by the
	// per-RPC log interceptor. Nil-safe (no-op when nil, e.g. in tests).
	Logger *zap.Logger
}

// NewConnectServer constructs a ConnectServer backed by app.
func NewConnectServer(app *app.Application) *ConnectServer {
	return &ConnectServer{App: app, Handlers: handler.New(app, nil)}
}

// Handler returns an http.Handler that serves all control plane services.
func (s *ConnectServer) Handler() http.Handler {
	mux := http.NewServeMux()
	interceptors := connect.WithInterceptors(
		tenantInterceptor(),
		obs.ConnectInterceptor(s.Logger), // W16+ Phase 5b: per-RPC log
	)
	_, h := v1connect.NewDomainRegistryServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.DomainRegistryService/", h)
	_, h = v1connect.NewSessionSchedulerServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.SessionSchedulerService/", h)
	_, h = v1connect.NewRuntimeDiscoveryServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.RuntimeDiscoveryService/", h)
	_, h = v1connect.NewTelemetryServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.TelemetryService/", h)
	_, h = v1connect.NewEvalServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.EvalService/", h)
	// SchedulerService lives on the same worker.proto as WorkerService but
	// is also exposed here so Python / CLI clients (Phase 1 client) can
	// drive pause/resume via gRPC over the same HTTP port as REST.
	_, h = v1connect.NewSchedulerServiceHandler(s, interceptors)
	mux.Handle("/hnsx.v1.SchedulerService/", h)
	return mux
}

func tenantInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			id := tenant.DefaultID
			if vals := req.Header().Values(tenant.HeaderName); len(vals) > 0 && vals[0] != "" {
				id = tenant.ID(vals[0])
			}
			return next(tenant.NewContext(ctx, id), req)
		}
	}
}

// DomainRegistryServiceHandler implementation.

func (s *ConnectServer) RegisterDomain(ctx context.Context, req *connect.Request[pb.RegisterDomainRequest]) (*connect.Response[pb.RegisterDomainResponse], error) {
	if s.App == nil || s.App.DomainService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("domain service unavailable"))
	}
	ds, err := domain.FromProto(req.Msg.GetSpec())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid domain spec: %w", err))
	}
	out, err := s.Handlers.RegisterDomainSpec(ctx, handler.RegisterDomainSpecInput{
		TenantID: tenant.FromContext(ctx),
		Spec:     ds,
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	return connect.NewResponse(&pb.RegisterDomainResponse{
		Domain: &pb.DomainRef{Id: out.Domain.ID, Version: out.Domain.Version},
	}), nil
}

func (s *ConnectServer) UnregisterDomain(ctx context.Context, req *connect.Request[pb.UnregisterDomainRequest]) (*connect.Response[pb.UnregisterDomainResponse], error) {
	err := s.Handlers.DeleteDomain(ctx, handler.DeleteDomainInput{
		TenantID: tenant.FromContext(ctx),
		ID:       req.Msg.GetDomain().GetId(),
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	return connect.NewResponse(&pb.UnregisterDomainResponse{}), nil
}

func (s *ConnectServer) GetDomain(ctx context.Context, req *connect.Request[pb.GetDomainRequest]) (*connect.Response[pb.GetDomainResponse], error) {
	out, err := s.Handlers.GetDomain(ctx, handler.GetDomainInput{
		TenantID: tenant.FromContext(ctx),
		ID:       req.Msg.GetDomain().GetId(),
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	pbSpec, err := domain.ToProto(out.Domain.Spec)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("convert domain spec: %w", err))
	}
	return connect.NewResponse(&pb.GetDomainResponse{Spec: pbSpec}), nil
}

func (s *ConnectServer) ListDomains(ctx context.Context, req *connect.Request[pb.ListDomainsRequest]) (*connect.Response[pb.ListDomainsResponse], error) {
	out, err := s.Handlers.ListDomains(ctx, handler.ListDomainsInput{
		TenantID: tenant.FromContext(ctx),
		Limit:    int(req.Msg.GetLimit()),
		Offset:   int(req.Msg.GetOffset()),
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	resp := &pb.ListDomainsResponse{Total: int32(out.Domains.Total)}
	for _, item := range out.Domains.Items {
		if item.Spec == nil {
			continue
		}
		pbSpec, err := domain.ToProto(item.Spec)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("convert domain spec: %w", err))
		}
		resp.Domains = append(resp.Domains, pbSpec)
	}
	return connect.NewResponse(resp), nil
}

func (s *ConnectServer) ValidateDomain(ctx context.Context, req *connect.Request[pb.ValidateDomainRequest]) (*connect.Response[pb.ValidateDomainResponse], error) {
	ds, err := domain.DecodeDomainSpec(strings.NewReader(req.Msg.GetDomainSpecJson()), "application/json")
	if err != nil {
		return connect.NewResponse(&pb.ValidateDomainResponse{
			Valid: false,
			Errors: []*pb.ValidationError{{Field: "", Message: err.Error()}},
		}), nil
	}
	if err := domain.Validate(ds); err != nil {
		return connect.NewResponse(&pb.ValidateDomainResponse{
			Valid: false,
			Errors: []*pb.ValidationError{{Field: "", Message: err.Error()}},
		}), nil
	}
	return connect.NewResponse(&pb.ValidateDomainResponse{Valid: true}), nil
}

// SessionSchedulerServiceHandler implementation.

func (s *ConnectServer) ScheduleSession(ctx context.Context, req *connect.Request[pb.ScheduleSessionRequest]) (*connect.Response[pb.ScheduleSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("SessionSchedulerService.ScheduleSession is deprecated; use SchedulerService.PullSession"))
}

// SchedulerServiceHandler — only PauseSession / ResumeSession are wired
// here. PullSession / AckSession / NackSession / StreamChannel continue
// to be served by the raw gRPC server in internal/controlplane/controlplane.go
// because they hold long-lived bidi streams that Connect doesn't model
// cleanly for workers.
func (s *ConnectServer) PauseSession(ctx context.Context, req *connect.Request[pb.PauseSessionRequest]) (*connect.Response[pb.PauseSessionResponse], error) {
	out, err := s.Handlers.PauseSession(ctx, handler.PauseSessionInput{
		TenantID:  tenant.FromContext(ctx),
		SessionID: req.Msg.GetSessionId(),
		Reason:    req.Msg.GetReason(),
	})
	if err != nil {
		return nil, mapSessionError(err)
	}
	return connect.NewResponse(&pb.PauseSessionResponse{
		Ok:           true,
		CurrentState: string(out.Session.State),
	}), nil
}

func (s *ConnectServer) ResumeSession(ctx context.Context, req *connect.Request[pb.ResumeSessionRequest]) (*connect.Response[pb.ResumeSessionResponse], error) {
	out, err := s.Handlers.ResumeSession(ctx, handler.ResumeSessionInput{
		TenantID:  tenant.FromContext(ctx),
		SessionID: req.Msg.GetSessionId(),
	})
	if err != nil {
		return nil, mapSessionError(err)
	}
	return connect.NewResponse(&pb.ResumeSessionResponse{
		Ok:           true,
		CurrentState: string(out.Session.State),
	}), nil
}

// Required by the generated SchedulerServiceHandler interface.
func (s *ConnectServer) PullSession(context.Context, *connect.Request[pb.PullSessionRequest]) (*connect.Response[pb.PullSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("PullSession is served over raw gRPC at :50061; not on this Connect mux"))
}

func (s *ConnectServer) AckSession(context.Context, *connect.Request[pb.AckSessionRequest]) (*connect.Response[pb.AckSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("AckSession is served over raw gRPC at :50061"))
}

func (s *ConnectServer) NackSession(context.Context, *connect.Request[pb.NackSessionRequest]) (*connect.Response[pb.NackSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("NackSession is served over raw gRPC at :50061"))
}

func (s *ConnectServer) StreamChannel(context.Context, *connect.BidiStream[pb.StreamChannelRequest, pb.StreamChannelResponse]) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("StreamChannel is served over raw gRPC at :50061"))
}

func (s *ConnectServer) GetSession(ctx context.Context, req *connect.Request[pb.GetSessionRequest]) (*connect.Response[pb.GetSessionResponse], error) {
	out, err := s.Handlers.GetSession(ctx, handler.GetSessionInput{
		TenantID:  tenant.FromContext(ctx),
		SessionID: req.Msg.GetSessionId(),
	})
	if err != nil {
		return nil, mapSessionError(err)
	}
	return connect.NewResponse(&pb.GetSessionResponse{Session: sessionDetailToProto(out.Session)}), nil
}

func (s *ConnectServer) ListSessions(ctx context.Context, req *connect.Request[pb.ListSessionsRequest]) (*connect.Response[pb.ListSessionsResponse], error) {
	out, err := s.Handlers.ListSessions(ctx, handler.ListSessionsInput{
		TenantID: tenant.FromContext(ctx),
		Filters: viewmodel.SessionFilters{
			DomainID: req.Msg.GetDomainId(),
			State:    req.Msg.GetState(),
		},
	})
	if err != nil {
		return nil, mapSessionError(err)
	}
	resp := &pb.ListSessionsResponse{Total: int32(out.Sessions.Total)}
	for _, sess := range out.Sessions.Items {
		resp.Sessions = append(resp.Sessions, sessionListItemToProto(sess))
	}
	return connect.NewResponse(resp), nil
}

// RuntimeDiscoveryServiceHandler implementation.

func (s *ConnectServer) DiscoverRuntime(ctx context.Context, req *connect.Request[pb.DiscoverRuntimeRequest]) (*connect.Response[pb.DiscoverRuntimeResponse], error) {
	out, err := s.Handlers.DiscoverRuntimes(ctx, handler.DiscoverRuntimesInput{
		TenantID:     tenant.FromContext(ctx),
		Capabilities: req.Msg.GetCapabilities(),
		Region:       req.Msg.GetRegion(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	runtimes := make([]*pb.RuntimeInfo, 0, len(out.Runtimes))
	for _, r := range out.Runtimes {
		runtimes = append(runtimes, &pb.RuntimeInfo{
			RuntimeId:    r.RuntimeID,
			Capabilities: r.Capabilities,
			Region:       r.Region,
			Version:      r.Version,
		})
	}
	return connect.NewResponse(&pb.DiscoverRuntimeResponse{Runtimes: runtimes}), nil
}

// TelemetryServiceHandler implementation.

func (s *ConnectServer) RecordTrace(ctx context.Context, req *connect.Request[pb.RecordTraceRequest]) (*connect.Response[pb.RecordTraceResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("trace service unavailable"))
	}
	obs := make([]domain.Observation, 0, len(req.Msg.GetObservations()))
	for _, o := range req.Msg.GetObservations() {
		obs = append(obs, observationToRuntime(o))
	}
	if err := s.Handlers.RecordTrace(ctx, handler.RecordTraceInput{
		TenantID:      tenant.FromContext(ctx),
		TraceID:       req.Msg.GetTraceId(),
		SessionID:     req.Msg.GetSessionId(),
		DomainID:      req.Msg.GetDomainId(),
		DomainVersion: req.Msg.GetDomainVersion(),
		Observations:  obs,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.RecordTraceResponse{}), nil
}

func (s *ConnectServer) QueryTraces(ctx context.Context, req *connect.Request[pb.QueryTracesRequest]) (*connect.Response[pb.QueryTracesResponse], error) {
	out, err := s.Handlers.QueryTraces(ctx, handler.QueryTracesInput{
		TenantID:  tenant.FromContext(ctx),
		TraceID:   req.Msg.GetTraceId(),
		DomainID:  req.Msg.GetDomainId(),
		SessionID: req.Msg.GetSessionId(),
		Limit:     int(req.Msg.GetLimit()),
	})
	if err != nil {
		if errors.Is(err, tracemodel.ErrTraceNotFound) {
			return connect.NewResponse(&pb.QueryTracesResponse{}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &pb.QueryTracesResponse{}
	for _, d := range out.Traces {
		resp.Traces = append(resp.Traces, traceDetailToProto(d))
	}
	return connect.NewResponse(resp), nil
}

func (s *ConnectServer) RecordInvocation(ctx context.Context, req *connect.Request[pb.RecordInvocationRequest]) (*connect.Response[pb.RecordInvocationResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("trace service unavailable"))
	}
	inv := req.Msg
	if err := s.Handlers.RecordInvocation(ctx, handler.RecordInvocationInput{
		TenantID:         tenant.FromContext(ctx),
		SessionID:        inv.GetSessionId(),
		DomainID:         inv.GetDomainId(),
		TotalCostUSD:     inv.GetTotalCostUsd(),
		PromptTokens:     inv.GetPromptTokens(),
		CompletionTokens: inv.GetCompletionTokens(),
		DurationMs:       inv.GetDurationMs(),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.RecordInvocationResponse{}), nil
}

func (s *ConnectServer) QueryInvocationMetrics(ctx context.Context, req *connect.Request[pb.QueryInvocationMetricsRequest]) (*connect.Response[pb.QueryInvocationMetricsResponse], error) {
	out, err := s.Handlers.QueryInvocationMetrics(ctx, handler.QueryInvocationMetricsInput{
		TenantID: tenant.FromContext(ctx),
		DomainID: req.Msg.GetDomainId(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.QueryInvocationMetricsResponse{
		DomainId:              out.DomainID,
		InvocationCount:       out.InvocationCount,
		TotalCostUsd:          out.TotalCostUSD,
		TotalPromptTokens:     out.TotalPromptTokens,
		TotalCompletionTokens: out.TotalCompletionTokens,
		AvgLatencyMs:          out.AvgLatencyMs,
	}), nil
}

// EvalServiceHandler implementation.

func (s *ConnectServer) RunEval(ctx context.Context, req *connect.Request[pb.RunEvalRequest]) (*connect.Response[pb.RunEvalResponse], error) {
	out, err := s.Handlers.RunEval(ctx, handler.RunEvalInput{
		TenantID:      tenant.FromContext(ctx),
		SetID:         req.Msg.GetSetId(),
		DomainID:      req.Msg.GetDomainId(),
		DomainVersion: req.Msg.GetDomainVersion(),
		Orchestration: req.Msg.GetOrchestration(),
		BaselineRunID: req.Msg.GetBaselineRunId(),
	})
	if err != nil {
		return nil, mapEvalError(err)
	}
	return connect.NewResponse(&pb.RunEvalResponse{EvalRunId: out.Run.RunID}), nil
}

func (s *ConnectServer) GetEvalRun(ctx context.Context, req *connect.Request[pb.GetEvalRunRequest]) (*connect.Response[pb.GetEvalRunResponse], error) {
	out, err := s.Handlers.GetEvalRun(ctx, handler.GetEvalRunInput{
		TenantID: tenant.FromContext(ctx),
		RunID:    req.Msg.GetEvalRunId(),
	})
	if err != nil {
		return nil, mapEvalError(err)
	}
	return connect.NewResponse(&pb.GetEvalRunResponse{Result: evalRunDetailToProto(out.Run)}), nil
}

func (s *ConnectServer) ListEvalRuns(ctx context.Context, req *connect.Request[pb.ListEvalRunsRequest]) (*connect.Response[pb.ListEvalRunsResponse], error) {
	out, err := s.Handlers.ListEvalRuns(ctx, handler.ListEvalRunsInput{
		TenantID: tenant.FromContext(ctx),
		SetID:    req.Msg.GetSetId(),
		Limit:    int(req.Msg.GetLimit()),
		Offset:   int(req.Msg.GetOffset()),
	})
	if err != nil {
		return nil, mapEvalError(err)
	}
	resp := &pb.ListEvalRunsResponse{Total: int32(out.Runs.Total)}
	for _, run := range out.Runs.Items {
		resp.Results = append(resp.Results, evalRunItemToProto(run))
	}
	return connect.NewResponse(resp), nil
}

// helpers.

func mapDomainError(err error) error {
	if errors.Is(err, domainmodel.ErrDomainNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, domainmodel.ErrDomainExists) {
		return connect.NewError(connect.CodeAlreadyExists, err)
	}
	if errors.Is(err, domainmodel.ErrInvalidSpec) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func mapSessionError(err error) error {
	if handler.IsSessionNotFound(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if handler.IsInvalidSession(err) || handler.IsAlreadyTerminal(err) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func sessionDetailToProto(sess *viewmodel.SessionDetail) *pb.SessionStatus {
	if sess == nil {
		return nil
	}
	started := sess.StartedAt.UnixMilli()
	var completed int64
	if sess.CompletedAt != nil {
		completed = sess.CompletedAt.UnixMilli()
	}
	var result string
	if sess.Result != nil {
		b, _ := json.Marshal(sess.Result)
		result = string(b)
	}
	return &pb.SessionStatus{
		SessionId:     sess.ID,
		DomainId:      sess.DomainID,
		DomainVersion: sess.DomainVersion,
		State:         sess.State,
		Result:        result,
		TraceId:       sess.ID,
		StartedAtMs:   started,
		CompletedAtMs: completed,
	}
}

func sessionListItemToProto(sess viewmodel.SessionListItem) *pb.SessionStatus {
	started := sess.StartedAt.UnixMilli()
	var completed int64
	if sess.CompletedAt != nil {
		completed = sess.CompletedAt.UnixMilli()
	}
	return &pb.SessionStatus{
		SessionId:     sess.ID,
		DomainId:      sess.DomainID,
		DomainVersion: sess.DomainVersion,
		State:         sess.State,
		TraceId:       sess.ID,
		StartedAtMs:   started,
		CompletedAtMs: completed,
	}
}

func observationToRuntime(obs *pb.Observation) domain.Observation {
	var payload, metadata map[string]any
	if obs.GetPayload() != "" {
		_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
	}
	if obs.GetMetadata() != "" {
		_ = json.Unmarshal([]byte(obs.GetMetadata()), &metadata)
	}
	return domain.Observation{
		Kind:      obs.GetKind(),
		SessionID: obs.GetSessionId(),
		DomainID:  obs.GetDomainId(),
		StepID:    obs.GetStepId(),
		AgentID:   obs.GetAgentId(),
		ParentID:  obs.GetParentId(),
		TraceID:   obs.GetTraceId(),
		Payload:   payload,
		Metadata:  metadata,
		Timestamp: time.UnixMilli(obs.GetCreatedAtMs()),
	}
}

func traceDetailToProto(d *viewmodel.TraceDetail) *pb.TraceRecord {
	if d == nil {
		return nil
	}
	rec := &pb.TraceRecord{
		TraceId:       d.TraceID,
		SessionId:     d.SessionID,
		DomainId:      d.DomainID,
		DomainVersion: d.DomainVersion,
	}
	for _, o := range d.Observations {
		rec.Observations = append(rec.Observations, observationItemToProto(o))
	}
	return rec
}

func observationItemToProto(o viewmodel.ObservationItem) *pb.Observation {
	payload, _ := toJSONString(o.Payload)
	metadata, _ := toJSONString(o.Metadata)
	return &pb.Observation{
		TraceId:       o.TraceID,
		SessionId:     o.SessionID,
		DomainId:      o.DomainID,
		DomainVersion: o.DomainVersion,
		StepId:        o.StepID,
		AgentId:       o.AgentID,
		Kind:          o.Kind,
		Payload:       payload,
		Metadata:      metadata,
		CreatedAtMs:   o.CreatedAt.UnixMilli(),
	}
}

func evalRunItemToProto(run viewmodel.EvalRunItem) *pb.EvalRunResult {
	return &pb.EvalRunResult{
		EvalRunId:     run.ID,
		DomainId:      run.DomainID,
		SetId:         run.EvalSetID,
		State:         run.State,
		Score:         run.Score,
		Total:         int32(run.Total),
		Passed:        int32(run.Passed),
		TotalCostUsd:  run.TotalCostUSD,
		DurationMs:    run.DurationMs,
		BaselineRunId: run.BaselineRunID,
	}
}

func evalRunDetailToProto(run *viewmodel.EvalRunDetail) *pb.EvalRunResult {
	if run == nil {
		return nil
	}
	cases := make([]*pb.EvalCaseResult, 0, len(run.Cases))
	for _, r := range run.Cases {
		actual, _ := toJSONString(r.Actual)
		details, _ := toJSONString(r.Details)
		cases = append(cases, &pb.EvalCaseResult{
			CaseId:    r.CaseID,
			SessionId: r.SessionID,
			Score:     r.Score,
			Passed:    r.Passed,
			Actual:    actual,
			Details:   details,
		})
	}
	return &pb.EvalRunResult{
		EvalRunId:     run.ID,
		DomainId:      run.DomainID,
		SetId:         run.EvalSetID,
		State:         run.State,
		Score:         run.Score,
		Total:         int32(run.Total),
		Passed:        int32(run.Passed),
		TotalCostUsd:  run.TotalCostUSD,
		DurationMs:    run.DurationMs,
		BaselineRunId: run.BaselineRunID,
		Cases:         cases,
	}
}

func mapEvalError(err error) error {
	if handler.IsEvalSetNotFound(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if handler.IsEvalRunNotFound(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if handler.IsDomainNotFound(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func toJSONString(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
