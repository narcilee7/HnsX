package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/auth"
	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

func newAuthTestServer(cfg config.AuthConfig) *Server {
	return &Server{
		App: &app.Application{
			Config: &config.Config{Auth: cfg},
		},
	}
}

// newAuthTestRouter returns a router with a no-op test endpoint under /api/v1/_test/auth
// so auth middleware can be exercised without wiring real services.
func newAuthTestRouter(s *Server) *gin.Engine {
	r := newRouter(s)
	r.GET("/api/v1/_test/auth", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestRouter_AuthMiddleware(t *testing.T) {
	secret := "router-test-secret"

	t.Run("jwt mode missing token returns 401", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "jwt", JWTSecret: secret}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("jwt mode malformed token returns 401", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "jwt", JWTSecret: secret}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("jwt mode bad signature returns 403", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "jwt", JWTSecret: secret}))
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "tenant-x",
			"role":      "operator",
		})
		signed, _ := token.SignedString([]byte("wrong-secret"))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		req.Header.Set("Authorization", "Bearer "+signed)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("jwt mode valid token is allowed", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "jwt", JWTSecret: secret}))
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "tenant-x",
			"role":      "operator",
		})
		signed, _ := token.SignedString([]byte(secret))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		req.Header.Set("Authorization", "Bearer "+signed)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("apikey mode unknown key returns 403", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{
			Mode: "apikey",
			APIKeys: map[string]config.APIKeyEntry{
				"known": {TenantID: "t1", Role: "operator"},
			},
		}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		req.Header.Set("X-HnsX-Api-Key", "unknown")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})

	t.Run("apikey mode valid key is allowed", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{
			Mode: "apikey",
			APIKeys: map[string]config.APIKeyEntry{
				"known": {TenantID: "t1", Role: "operator"},
			},
		}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		req.Header.Set("X-HnsX-Api-Key", "known")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("none mode allows unauthenticated requests", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "none"}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("invalid auth mode returns 500", func(t *testing.T) {
		r := newAuthTestRouter(newAuthTestServer(config.AuthConfig{Mode: "oauth2"}))
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/auth", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
	})
}

func TestRouter_RequireRole(t *testing.T) {
	s := newAuthTestServer(config.AuthConfig{Mode: "none"})
	r := newRouter(s)
	// Add test-only endpoints protected by requireRole so we do not need
	// real approval/session dependencies.
	r.GET("/api/v1/_test/operator-only", requireRole(auth.RoleOperator), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/_test/admin-only", requireRole(auth.RolePlatformAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	t.Run("operator role can access operator endpoint", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/operator-only", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	t.Run("default operator role cannot access admin endpoint", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/admin-only", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", w.Code)
		}
	})
}

func TestRouter_TenantMiddleware(t *testing.T) {
	s := newAuthTestServer(config.AuthConfig{
		Mode: "apikey",
		APIKeys: map[string]config.APIKeyEntry{
			"tenant-a": {TenantID: "tenant-a", Role: "operator"},
		},
	})
	r := newRouter(s)
	var captured tenant.ID
	r.GET("/api/v1/_test/tenant", func(c *gin.Context) {
		captured = tenantFromGin(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/_test/tenant", nil)
	req.Header.Set("X-HnsX-Api-Key", "tenant-a")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if captured != tenant.ID("tenant-a") {
		t.Fatalf("tenant = %q, want tenant-a", captured)
	}
}
