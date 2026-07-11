package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// newRouter mounts the full API surface onto a gin engine.
func newRouter(s *Server) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware.
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(tenantMiddleware())
	r.Use(metricsMiddleware())
	r.Use(drainMiddleware(s))

	// Health (no /api/v1 prefix per convention).
	r.GET("/healthz", s.Health)
	r.GET("/readyz", s.Readiness)

	// Connect-RPC control plane (served before /api/v1 so it gets middleware).
	if s.ConnectHandler != nil {
		for _, svc := range []string{
			"DomainRegistryService",
			"SessionSchedulerService",
			"RuntimeDiscoveryService",
			"TelemetryService",
			"EvalService",
		} {
			r.Any("/hnsx.v1."+svc+"/*path", gin.WrapH(s.ConnectHandler))
		}
	}

	// Versioned API.
	v1 := r.Group("/api/v1")
	{
		domains := v1.Group("/domains")
		{
			domains.GET("", s.ListDomains)
			domains.POST("", s.RegisterDomain)
			d := domains.Group("/:id")
			{
				d.GET("", s.GetDomain)
				d.GET("/yaml", s.GetDomainYAML)
				d.PUT("", s.UpdateDomain)
				d.DELETE("", s.DeleteDomain)
				d.GET("/versions", s.ListDomainVersions)
				d.GET("/versions/:version", s.GetDomainVersion)
				d.GET("/schema", s.GetDomainSchema)
				d.POST("/validate", s.ValidateDomain)
				d.POST("/run", s.TriggerDomain)
				d.POST("/policies", s.BindPolicy)
			}
		}

		sessions := v1.Group("/sessions")
		{
			sessions.GET("", s.ListSessions)
			sessions.POST("", s.TriggerSession)
			sg := sessions.Group("/:id")
			{
				sg.GET("", s.GetSession)
				sg.GET("/trace", s.GetSessionTrace)
				sg.GET("/events", s.StreamSessionEvents)
				sg.POST("/cancel", s.CancelSession)
				sg.POST("/rerun", s.RerunSession)
			}
		}

		traces := v1.Group("/traces")
		{
			traces.GET("", s.ListTraces)
			traces.GET("/:traceId", s.GetTrace)
		}

		approvals := v1.Group("/approvals")
		{
			approvals.GET("", s.ListApprovals)
			approvals.POST("", s.CreateApproval)
			ap := approvals.Group("/:id")
			{
				ap.GET("", s.GetApproval)
				ap.POST("/approve", s.ApproveApproval)
				ap.POST("/reject", s.RejectApproval)
			}
		}

		evals := v1.Group("/evals")
		{
			evals.GET("", s.ListEvalSets)
			evals.POST("", s.CreateEvalSet)
			e := evals.Group("/:setId")
			{
				e.GET("", s.GetEvalSet)
				e.PUT("", s.UpdateEvalSet)
				e.DELETE("", s.DeleteEvalSet)
				e.POST("/run", s.RunEval)
				e.GET("/runs", s.ListEvalRuns)
				e.GET("/runs/:runId", s.GetEvalRun)
			}
		}

		v1.GET("/audit", s.ListAudit)
		v1.GET("/metrics", s.GetMetrics)
		v1.GET("/runtimes", s.ListRuntimes)
		v1.GET("/templates", s.ListTemplates)

		secrets := v1.Group("/secrets")
		{
			secrets.GET("", s.ListSecrets)
			secrets.POST("", s.CreateSecret)
			sc := secrets.Group("/:id")
			{
				sc.PUT("", s.UpdateSecret)
				sc.DELETE("", s.DeleteSecret)
			}
		}

		v1.GET("/policies", s.ListPolicies)
		policies := v1.Group("/policies")
		{
			policies.POST("", s.CreatePolicy)
			policies.PUT("/:id", s.UpdatePolicy)
			policies.DELETE("/:id", s.DeletePolicy)
		}
	}

	return r
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization, X-HnsX-Api-Key, X-Tenant-ID")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func tenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := tenant.ID(c.GetHeader(tenant.HeaderName))
		if id == "" {
			id = tenant.DefaultID
		}
		c.Set(tenantContextKey, id)
		c.Next()
	}
}

func tenantFromGin(c *gin.Context) tenant.ID {
	if v, ok := c.Get(tenantContextKey); ok {
		if id, ok := v.(tenant.ID); ok {
			return id
		}
	}
	return tenant.DefaultID
}

const tenantContextKey = "hnsx-tenant-id"

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-HnsX-Route", c.Request.URL.Path)
		c.Next()
	}
}

func drainMiddleware(s *Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.IsDraining() {
			c.Header("Retry-After", "0")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, APIError{
				Code:    "SERVICE_UNAVAILABLE",
				Message: "server is shutting down",
			})
			return
		}
		s.TrackRequest()
		defer s.DoneRequest()
		c.Next()
	}
}
