// hnsx-server is the HnsX control plane daemon. It hosts the HTTP/REST API
// and the gRPC control plane for Python Runtime Workers.
//
// Usage:
//
//	hnsx-server server [--config <path>]
//	hnsx-server version
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	stdruntime "runtime"
	"syscall"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/config"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/api"
	"github.com/hnsx-io/hnsx/server/pkg/controlplane"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/version"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		os.Exit(cmdServer(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version.String())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`hnsx-server — HnsX Control Plane

Usage:
  hnsx-server server [--config <path>]
  hnsx-server version

Environment:
  HNSX_HTTP_ADDR           Listen address (default 127.0.0.1:50051)
  HNSX_DATABASE_URL        Postgres connection string
  HNSX_MIGRATIONS_DIR      SQL migrations directory
  HNSX_OTEL_EXPORTER       stdout | otlp | none
  HNSX_OTEL_OTLP_ENDPOINT  OTLP gRPC endpoint (default 127.0.0.1:4317)
  HNSX_OTEL_SERVICE_NAME   service.name attribute
  HNSX_LOG_LEVEL           debug | info | warn | error
  HNSX_REDIS_ADDR          Redis address for the session queue (e.g. 127.0.0.1:6379)
  HNSX_REDIS_PASSWORD      Redis AUTH password
  HNSX_REDIS_DB            Redis logical database number
  HNSX_REDIS_QUEUE_PREFIX  Redis key prefix for the queue (default hnsx:queue)
`)
}

func cmdServer(args []string) int {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "optional path to YAML config")
	seedFrom := fs.String("seed-from", "", "optional directory of v2 DomainSpec YAMLs to register on boot (development only; production deployments register via POST /api/v1/domains)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application, err := app.NewApplication(ctx, cfg)
	if err != nil {
		log.Fatalf("application: %v", err)
	}
	defer func() {
		shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		if err := application.Close(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] application close: %v", err)
		}
	}()

	build := api.BuildInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		Built:     version.Built,
		GoVersion: stdruntime.Version(),
	}

	srv := api.NewServerWithWorkerPool(build, application)

	if *seedFrom != "" {
		seedFromDir(srv, *seedFrom)
	}

	var grpcSrv *controlplane.Server
	if cfg.GRPCAddr != "" {
		grpcSrv = controlplane.NewServer(cfg.GRPCAddr).WithWorkerServices(application.WorkerRegistry, application.SessionQueue)
		if grpcSrv.Sched != nil {
			grpcSrv.Sched.OnObservation = func(tid tenant.ID, sessionID string, obs *pb.Observation) {
				payload := map[string]any{}
				if obs.GetPayload() != "" {
					_ = json.Unmarshal([]byte(obs.GetPayload()), &payload)
				}
				srv.PublishObservation(sessionID, runtime.Observation{
					Kind:      obs.GetKind(),
					SessionID: obs.GetSessionId(),
					DomainID:  obs.GetDomainId(),
					StepID:    obs.GetStepId(),
					AgentID:   obs.GetAgentId(),
					ParentID:  obs.GetParentId(),
					TraceID:   obs.GetTraceId(),
					Payload:   payload,
					Timestamp: time.UnixMilli(obs.GetCreatedAtMs()),
				})
			}
			grpcSrv.Sched.OnSessionStatus = func(tid tenant.ID, sessionID, state string) {
				if application.SessionService != nil {
					_, _ = application.SessionService.UpdateState(sessionID, sessionmodel.State(state))
				}
			}
		}
	}

	// Stale-worker GC: every 30s, evict workers that haven't heartbeat
	// in over 60s and log how many were reaped. Only runs when worker pool
	// is enabled.
	stopGC := make(chan struct{})
	if application.WorkerService != nil {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopGC:
					return
				case <-ticker.C:
					if evicted := application.WorkerService.EvictStale(60 * time.Second); len(evicted) > 0 {
						log.Printf("[hnsx-server] evicted %d stale worker(s): %v", len(evicted), evicted)
						if grpcSrv != nil && grpcSrv.Sched != nil {
							for _, wid := range evicted {
								if requeued := grpcSrv.Sched.RequeueSessions(wid); len(requeued) > 0 {
									log.Printf("[hnsx-server] requeued %d session(s) from worker %s", len(requeued), wid)
								}
							}
						}
					}
				}
			}
		}()
	}

	log.Printf("[hnsx-server] listening on http=%s grpc=%s", cfg.HTTPAddr, cfg.GRPCAddr)
	log.Printf("[hnsx-server] version=%s commit=%s", build.Version, build.Commit)
	log.Printf("[hnsx-server] otel=%s build=%s", cfg.OTel.Exporter, build.GoVersion)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Listen(cfg.HTTPAddr)
	}()

	var grpcErr chan error
	if grpcSrv != nil {
		grpcErr = make(chan error, 1)
		go func() {
			grpcErr <- grpcSrv.ListenAndServe(ctx)
		}()
	}

	select {
	case <-ctx.Done():
		log.Println("[hnsx-server] shutting down")
		if application.WorkerService != nil {
			close(stopGC)
		}
		shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel2()
		if err := srv.Drain(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] api drain: %v", err)
		}
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[hnsx-server] api shutdown: %v", err)
		}
		return 0
	case err := <-serveErr:
		log.Fatalf("http: %v", err)
		return 1
	case err := <-grpcErr:
		log.Fatalf("grpc: %v", err)
		return 1
	}
}

// seedFromDir walks the given directory and registers every v2 DomainSpec YAML
// against the API server. It is an explicit operator action (--seed-from) so
// production deployments never implicitly pull in development fixtures.
//
// Each subdirectory of dir is expected to contain a single `domain.yaml`. Any
// YAML that fails validation is logged and skipped — the server still boots.
func seedFromDir(s *api.Server, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("[seed] cannot read %s: %v (skipping)", dir, err)
		return
	}
	registered := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := fmt.Sprintf("%s/%s/domain.yaml", dir, e.Name())
		spec, err := spec.LoadFile(path)
		if err != nil {
			log.Printf("[seed] skip %s: %v", path, err)
			continue
		}
		s.RegisterBootstrapDomain(tenant.DefaultID, spec)
		registered++
	}
	if registered > 0 {
		log.Printf("[seed] registered %d domain(s) from %s", registered, dir)
	}
}
