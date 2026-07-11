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
	"time"

	"connectrpc.com/connect"

	"github.com/google/uuid"
	"github.com/hnsx-io/hnsx/server/internal/app"
	domainmodel "github.com/hnsx-io/hnsx/server/internal/domain/model"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrunner "github.com/hnsx-io/hnsx/server/internal/evaluation/runner"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1/v1connect"
)

// ConnectServer implements the 5 control_plane.proto services.
type ConnectServer struct {
	App *app.Application
}

// NewConnectServer constructs a ConnectServer backed by app.
func NewConnectServer(app *app.Application) *ConnectServer {
	return &ConnectServer{App: app}
}

// Handler returns an http.Handler that serves all 5 control plane services.
func (s *ConnectServer) Handler() http.Handler {
	mux := http.NewServeMux()
	interceptors := connect.WithInterceptors(tenantInterceptor())
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
	ds, err := spec.FromProto(req.Msg.GetSpec())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid domain spec: %w", err))
	}
	d, err := s.App.DomainService.Register(tenant.FromContext(ctx), ds)
	if err != nil {
		return nil, mapDomainError(err)
	}
	return connect.NewResponse(&pb.RegisterDomainResponse{
		Domain: &pb.DomainRef{Id: d.ID, Version: d.Version},
	}), nil
}

func (s *ConnectServer) UnregisterDomain(ctx context.Context, req *connect.Request[pb.UnregisterDomainRequest]) (*connect.Response[pb.UnregisterDomainResponse], error) {
	if s.App == nil || s.App.DomainService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("domain service unavailable"))
	}
	ref := req.Msg.GetDomain()
	if err := s.App.DomainService.Delete(tenant.FromContext(ctx), ref.GetId()); err != nil {
		return nil, mapDomainError(err)
	}
	return connect.NewResponse(&pb.UnregisterDomainResponse{}), nil
}

func (s *ConnectServer) GetDomain(ctx context.Context, req *connect.Request[pb.GetDomainRequest]) (*connect.Response[pb.GetDomainResponse], error) {
	if s.App == nil || s.App.DomainService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("domain service unavailable"))
	}
	d, err := s.App.DomainService.Get(tenant.FromContext(ctx), req.Msg.GetDomain().GetId())
	if err != nil {
		return nil, mapDomainError(err)
	}
	pbSpec, err := spec.ToProto(d.Spec)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("convert domain spec: %w", err))
	}
	return connect.NewResponse(&pb.GetDomainResponse{Spec: pbSpec}), nil
}

func (s *ConnectServer) ListDomains(ctx context.Context, req *connect.Request[pb.ListDomainsRequest]) (*connect.Response[pb.ListDomainsResponse], error) {
	if s.App == nil || s.App.DomainService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("domain service unavailable"))
	}
	items, err := s.App.DomainService.List(tenant.FromContext(ctx))
	if err != nil {
		return nil, mapDomainError(err)
	}
	resp := &pb.ListDomainsResponse{Total: int32(len(items))}
	for _, d := range items {
		pbSpec, err := spec.ToProto(d.Spec)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("convert domain spec: %w", err))
		}
		resp.Domains = append(resp.Domains, pbSpec)
	}
	return connect.NewResponse(resp), nil
}

// SessionSchedulerServiceHandler implementation.

func (s *ConnectServer) ScheduleSession(ctx context.Context, req *connect.Request[pb.ScheduleSessionRequest]) (*connect.Response[pb.ScheduleSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("SessionSchedulerService.ScheduleSession is deprecated; use SchedulerService.PullSession"))
}

func (s *ConnectServer) GetSession(ctx context.Context, req *connect.Request[pb.GetSessionRequest]) (*connect.Response[pb.GetSessionResponse], error) {
	if s.App == nil || s.App.SessionService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("session service unavailable"))
	}
	sess, err := s.App.SessionService.Get(tenant.FromContext(ctx), req.Msg.GetSessionId())
	if err != nil {
		return nil, mapSessionError(err)
	}
	return connect.NewResponse(&pb.GetSessionResponse{Session: sessionToProto(sess)}), nil
}

func (s *ConnectServer) ListSessions(ctx context.Context, req *connect.Request[pb.ListSessionsRequest]) (*connect.Response[pb.ListSessionsResponse], error) {
	if s.App == nil || s.App.SessionService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("session service unavailable"))
	}
	tid := tenant.FromContext(ctx)
	var items []*sessionmodel.Session
	var err error
	if req.Msg.GetDomainId() != "" {
		items, err = s.App.SessionService.ListByDomain(tid, req.Msg.GetDomainId())
	} else {
		items, err = s.App.SessionService.List(tid)
	}
	if err != nil {
		return nil, mapSessionError(err)
	}
	stateFilter := req.Msg.GetState()
	resp := &pb.ListSessionsResponse{}
	for _, sess := range items {
		if stateFilter != "" && string(sess.State) != stateFilter {
			continue
		}
		resp.Sessions = append(resp.Sessions, sessionToProto(sess))
	}
	resp.Total = int32(len(resp.Sessions))
	return connect.NewResponse(resp), nil
}

// RuntimeDiscoveryServiceHandler implementation.

func (s *ConnectServer) DiscoverRuntime(ctx context.Context, req *connect.Request[pb.DiscoverRuntimeRequest]) (*connect.Response[pb.DiscoverRuntimeResponse], error) {
	if s.App == nil || s.App.WorkerService == nil {
		return connect.NewResponse(&pb.DiscoverRuntimeResponse{}), nil
	}
	var runtimes []*pb.RuntimeInfo
	required := req.Msg.GetCapabilities()
	region := req.Msg.GetRegion()
	for _, w := range s.App.WorkerService.List() {
		info := w.Info
		if info == nil {
			continue
		}
		if region != "" && info.GetRegion() != region {
			continue
		}
		caps := workerCapabilities(info)
		if !hasAll(caps, required) {
			continue
		}
		runtimes = append(runtimes, &pb.RuntimeInfo{
			RuntimeId:    w.WorkerID,
			Capabilities: caps,
			Region:       info.GetRegion(),
			Version:      info.GetVersion(),
		})
	}
	return connect.NewResponse(&pb.DiscoverRuntimeResponse{Runtimes: runtimes}), nil
}

// TelemetryServiceHandler implementation.

func (s *ConnectServer) RecordTrace(ctx context.Context, req *connect.Request[pb.RecordTraceRequest]) (*connect.Response[pb.RecordTraceResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("trace service unavailable"))
	}
	for _, obs := range req.Msg.GetObservations() {
		ro := observationToRuntime(obs)
		if err := s.App.TraceService.Record(ctx, ro); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	return connect.NewResponse(&pb.RecordTraceResponse{}), nil
}

func (s *ConnectServer) QueryTraces(ctx context.Context, req *connect.Request[pb.QueryTracesRequest]) (*connect.Response[pb.QueryTracesResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return connect.NewResponse(&pb.QueryTracesResponse{}), nil
	}
	resp := &pb.QueryTracesResponse{}
	if req.Msg.GetTraceId() != "" {
		detail, err := s.App.TraceService.Detail(req.Msg.GetTraceId())
		if err != nil {
			if errors.Is(err, model.ErrTraceNotFound) {
				return connect.NewResponse(resp), nil
			}
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Traces = append(resp.Traces, traceDetailToProto(detail))
		return connect.NewResponse(resp), nil
	}
	svc := s.App.TraceService
	filter := model.TraceListFilter{
		DomainID:  req.Msg.GetDomainId(),
		SessionID: req.Msg.GetSessionId(),
		Limit:     int(req.Msg.GetLimit()),
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	summaries, err := svc.ListSummaries(filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, sum := range summaries.Summaries {
		resp.Traces = append(resp.Traces, traceSummaryToProto(sum))
	}
	return connect.NewResponse(resp), nil
}

func (s *ConnectServer) RecordInvocation(ctx context.Context, req *connect.Request[pb.RecordInvocationRequest]) (*connect.Response[pb.RecordInvocationResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("trace service unavailable"))
	}
	inv := req.Msg
	obs := runtime.Observation{
		Kind:          "invocation",
		SessionID:     inv.GetSessionId(),
		DomainID:      inv.GetDomainId(),
		TraceID:       inv.GetSessionId(),
		Timestamp:     time.Now().UTC(),
		Cost: &runtime.Cost{
			CostUSD:          inv.GetTotalCostUsd(),
			PromptTokens:     int(inv.GetPromptTokens()),
			CompletionTokens: int(inv.GetCompletionTokens()),
			LatencyMs:        inv.GetDurationMs(),
		},
	}
	if err := s.App.TraceService.Record(ctx, obs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.RecordInvocationResponse{}), nil
}

func (s *ConnectServer) QueryInvocationMetrics(ctx context.Context, req *connect.Request[pb.QueryInvocationMetricsRequest]) (*connect.Response[pb.QueryInvocationMetricsResponse], error) {
	if s.App == nil || s.App.TraceService == nil {
		return connect.NewResponse(&pb.QueryInvocationMetricsResponse{DomainId: req.Msg.GetDomainId()}), nil
	}
	tid := tenant.FromContext(ctx)
	domainID := req.Msg.GetDomainId()
	var sessionIDs []string
	if domainID != "" && s.App.SessionService != nil {
		sessions, err := s.App.SessionService.ListByDomain(tid, domainID)
		if err == nil {
			for _, sess := range sessions {
				sessionIDs = append(sessionIDs, sess.ID)
			}
		}
	}
	agg, err := s.App.TraceService.Aggregate(sessionIDs)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var avgLatency float64
	count := agg.AgentInvocations + agg.ToolInvocations
	if count > 0 {
		avgLatency = 0 // TODO: trace aggregate does not yet expose avg latency
	}
	return connect.NewResponse(&pb.QueryInvocationMetricsResponse{
		DomainId:              domainID,
		InvocationCount:       int64(count),
		TotalCostUsd:          agg.TotalCostUSD,
		TotalPromptTokens:     int64(agg.TotalPromptTokens),
		TotalCompletionTokens: int64(agg.TotalCompletionTokens),
		AvgLatencyMs:          avgLatency,
	}), nil
}

// EvalServiceHandler implementation.

func (s *ConnectServer) RunEval(ctx context.Context, req *connect.Request[pb.RunEvalRequest]) (*connect.Response[pb.RunEvalResponse], error) {
	if s.App == nil || s.App.EvalService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("eval service unavailable"))
	}
	if s.App.Executor == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("eval runner requires the local executor"))
	}
	tid := tenant.FromContext(ctx)
	set, err := s.App.EvalService.GetSet(req.Msg.GetSetId())
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	domainID := req.Msg.GetDomainId()
	if domainID == "" {
		domainID = set.DomainID
	}
	domain, err := s.App.DomainService.Get(tid, domainID)
	if err != nil {
		return nil, mapDomainError(err)
	}
	run := &evalmodel.EvalRun{
		ID:            uuid.NewString(),
		EvalSetID:     set.ID,
		DomainID:      domainID,
		DomainVersion: req.Msg.GetDomainVersion(),
		Orchestration: req.Msg.GetOrchestration(),
		BaselineRunID: req.Msg.GetBaselineRunId(),
		State:         "running",
		TotalCases:    len(set.Cases),
	}
	if run.DomainVersion == "" {
		run.DomainVersion = domain.Version
	}
	if run.Orchestration == "" && domain.Spec != nil {
		run.Orchestration = string(domain.Spec.Harness.Session.Mode)
	}
	if err := s.App.EvalService.CreateRun(run); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	budget := 0.0
	if domain.Spec != nil {
		budget = domain.Spec.Harness.Policy.Budget.MaxCostUSD
	}
	traceSvc := s.App.TraceService
	er := evalrunner.New(s.App.Executor, s.App.EvalService, evalrunner.WithCostFunc(func(sessionID string) float64 {
		if traceSvc == nil {
			return 0
		}
		agg, err := traceSvc.Aggregate([]string{sessionID})
		if err != nil {
			return 0
		}
		return agg.TotalCostUSD
	}))
	go func() {
		_ = er.Run(ctx, run, set, domain.Spec, budget)
	}()

	return connect.NewResponse(&pb.RunEvalResponse{EvalRunId: run.ID}), nil
}

func (s *ConnectServer) GetEvalRun(ctx context.Context, req *connect.Request[pb.GetEvalRunRequest]) (*connect.Response[pb.GetEvalRunResponse], error) {
	if s.App == nil || s.App.EvalService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("eval service unavailable"))
	}
	run, err := s.App.EvalService.GetRun(req.Msg.GetEvalRunId())
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalRunNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.GetEvalRunResponse{Result: evalRunToProto(run)}), nil
}

func (s *ConnectServer) ListEvalRuns(ctx context.Context, req *connect.Request[pb.ListEvalRunsRequest]) (*connect.Response[pb.ListEvalRunsResponse], error) {
	if s.App == nil || s.App.EvalService == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("eval service unavailable"))
	}
	runs, err := s.App.EvalService.RunsBySet(req.Msg.GetSetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp := &pb.ListEvalRunsResponse{Total: int32(len(runs))}
	for _, run := range runs {
		resp.Results = append(resp.Results, evalRunToProto(&run))
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
	if errors.Is(err, sessionmodel.ErrSessionNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, sessionmodel.ErrInvalidSession) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func sessionToProto(sess *sessionmodel.Session) *pb.SessionStatus {
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
		b, _ := json.Marshal(sess.Result.Output)
		result = string(b)
	}
	return &pb.SessionStatus{
		SessionId:     sess.ID,
		DomainId:      sess.DomainID,
		DomainVersion: sess.DomainVersion,
		State:         string(sess.State),
		Result:        result,
		TraceId:       sess.ID,
		StartedAtMs:   started,
		CompletedAtMs: completed,
	}
}

func workerCapabilities(info *pb.WorkerInfo) []string {
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

func hasAll(have, want []string) bool {
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

func observationToRuntime(obs *pb.Observation) runtime.Observation {
	var payload, metadata map[string]any
	if obs.GetPayload() != "" {
		_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
	}
	if obs.GetMetadata() != "" {
		_ = json.Unmarshal([]byte(obs.GetMetadata()), &metadata)
	}
	return runtime.Observation{
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

func traceDetailToProto(d *model.TraceDetail) *pb.TraceRecord {
	rec := traceSummaryToProto(d.TraceSummary)
	for _, o := range d.Observations {
		rec.Observations = append(rec.Observations, observationRecordToProto(o))
	}
	return rec
}

func traceSummaryToProto(s model.TraceSummary) *pb.TraceRecord {
	return &pb.TraceRecord{
		TraceId:       s.TraceID,
		SessionId:     s.SessionID,
		DomainId:      s.DomainID,
		DomainVersion: s.DomainVersion,
	}
}

func observationRecordToProto(o model.ObservationRecord) *pb.Observation {
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

func evalRunToProto(run *evalmodel.EvalRun) *pb.EvalRunResult {
	cases := make([]*pb.EvalCaseResult, 0, len(run.Results))
	for _, r := range run.Results {
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
		Total:         int32(run.TotalCases),
		Passed:        int32(run.PassedCases),
		TotalCostUsd:  run.TotalCostUSD,
		DurationMs:    run.DurationMs,
		BaselineRunId: run.BaselineRunID,
		Cases:         cases,
	}
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
