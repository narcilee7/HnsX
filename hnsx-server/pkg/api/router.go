package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// newRouter mounts the full API surface onto a chi router. Routes follow the
// contract documented in docs/server-design/api-design.md.
func newRouter(s *Server) http.Handler {
	r := chi.NewRouter()

	// Middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(metricsMiddleware)

	// Health (no /api/v1 prefix per convention).
	r.Get("/healthz", s.Health)
	r.Get("/readyz", s.Readiness)

	// Versioned API.
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/domains", func(r chi.Router) {
			r.Get("/", s.ListDomains)
			r.Post("/", s.RegisterDomain)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", s.GetDomain)
				r.Put("/", s.UpdateDomain)
				r.Delete("/", s.DeleteDomain)
				r.Get("/versions", s.ListDomainVersions)
				r.Post("/validate", s.ValidateDomain)
				r.Post("/run", s.TriggerDomain)
			})
		})

		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", s.ListSessions)
			r.Post("/", s.TriggerSession)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", s.GetSession)
				r.Get("/trace", s.GetSessionTrace)
				r.Get("/events", s.StreamSessionEvents)
				r.Post("/cancel", s.CancelSession)
				r.Post("/rerun", s.RerunSession)
			})
		})

		r.Route("/traces", func(r chi.Router) {
			r.Get("/", s.ListTraces)
			r.Get("/{traceId}", s.GetTrace)
		})

		r.Route("/approvals", func(r chi.Router) {
			r.Get("/", s.ListApprovals)
			r.Route("/{id}", func(r chi.Router) {
				r.Post("/approve", s.ApproveApproval)
				r.Post("/reject", s.RejectApproval)
			})
		})

		r.Route("/evals", func(r chi.Router) {
			r.Get("/", s.ListEvalSets)
			r.Post("/", s.CreateEvalSet)
			r.Route("/{setId}", func(r chi.Router) {
				r.Get("/", s.GetEvalSet)
				r.Post("/run", s.RunEval)
				r.Route("/runs/{runId}", func(r chi.Router) {
					r.Get("/", s.GetEvalRun)
				})
			})
		})

		r.Route("/audit", func(r chi.Router) {
			r.Get("/", s.ListAudit)
		})

		r.Route("/metrics", func(r chi.Router) {
			r.Get("/", s.GetMetrics)
		})

		r.Route("/runtimes", func(r chi.Router) {
			r.Get("/", s.ListRuntimes)
		})

		r.Route("/secrets", func(r chi.Router) {
			r.Get("/", s.ListSecrets)
			r.Post("/", s.CreateSecret)
			r.Route("/{id}", func(r chi.Router) {
				r.Put("/", s.UpdateSecret)
				r.Delete("/", s.DeleteSecret)
			})
		})

		r.Route("/policies", func(r chi.Router) {
			r.Get("/", s.ListPolicies)
		})
	})

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization, X-HnsX-Api-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// metricsMiddleware is a placeholder for future per-route Prometheus metrics.
// Phase 1 keeps it cheap — it just stamps a header so callers can verify.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-HnsX-Route", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
