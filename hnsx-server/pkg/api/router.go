package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/auth"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// newRouter mounts the full API surface onto a gin engine.
func newRouter(s *Server) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Global middleware.
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())
	r.Use(authMiddleware(s))
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
				ap.POST("/approve", requireRole(auth.RoleOperator, auth.RolePlatformAdmin), s.ApproveApproval)
				ap.POST("/reject", requireRole(auth.RoleOperator, auth.RolePlatformAdmin), s.RejectApproval)
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
			secrets.POST("", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.CreateSecret)
			sc := secrets.Group("/:id")
			{
				sc.PUT("", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.UpdateSecret)
				sc.DELETE("", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.DeleteSecret)
			}
		}

		v1.GET("/policies", s.ListPolicies)
		policies := v1.Group("/policies")
		{
			policies.POST("", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.CreatePolicy)
			policies.PUT("/:id", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.UpdatePolicy)
			policies.DELETE("/:id", requireRole(auth.RolePlatformAdmin, auth.RoleHarnessDesigner), s.DeletePolicy)
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

// authMiddleware enforces authentication and maps the caller to a tenant/role.
// In "none" mode it still sets the default identity so downstream code always
// sees a valid tenant and role.
func authMiddleware(s *Server) gin.HandlerFunc {
	authenticator, err := auth.NewAuthenticator(&s.App.Config.Auth)
	return func(c *gin.Context) {
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, APIError{
				Code:    "AUTH_NOT_CONFIGURED",
				Message: err.Error(),
			})
			return
		}

		identity, aerr := authenticator.Authenticate(c.Request)
		if aerr != nil {
			code := "UNAUTHORIZED"
			status := http.StatusUnauthorized
			if errors.Is(aerr, auth.ErrForbidden) {
				code = "FORBIDDEN"
				status = http.StatusForbidden
			}
			c.AbortWithStatusJSON(status, APIError{Code: code, Message: aerr.Error()})
			return
		}

		c.Set(authContextKey, identity)
		c.Request = c.Request.WithContext(auth.NewContext(c.Request.Context(), identity))
		c.Next()
	}
}

// requireRole returns a middleware that aborts with 403 unless the caller has
// one of the supplied roles.
func requireRole(roles ...auth.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !auth.HasRole(c.Request.Context(), roles...) {
			c.AbortWithStatusJSON(http.StatusForbidden, APIError{
				Code:    "FORBIDDEN",
				Message: "insufficient permissions",
			})
			return
		}
		c.Next()
	}
}

func tenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var id tenant.ID
		if identity := auth.FromContext(c.Request.Context()); identity != nil {
			id = identity.TenantID
		}
		if id == "" {
			id = tenant.ID(c.GetHeader(tenant.HeaderName))
		}
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
const authContextKey = "hnsx-auth-identity"

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
