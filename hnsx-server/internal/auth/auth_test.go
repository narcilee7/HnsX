package auth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

func TestNewAuthenticator(t *testing.T) {
	t.Run("nil config defaults to none", func(t *testing.T) {
		a, err := NewAuthenticator(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := a.(*NoneAuthenticator); !ok {
			t.Fatalf("expected NoneAuthenticator, got %T", a)
		}
	})

	t.Run("none mode", func(t *testing.T) {
		a, err := NewAuthenticator(&config.AuthConfig{Mode: "none"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := a.(*NoneAuthenticator); !ok {
			t.Fatalf("expected NoneAuthenticator, got %T", a)
		}
	})

	t.Run("jwt mode requires secret", func(t *testing.T) {
		_, err := NewAuthenticator(&config.AuthConfig{Mode: "jwt"})
		if err == nil {
			t.Fatal("expected error for missing jwt secret")
		}
	})

	t.Run("jwt mode with secret", func(t *testing.T) {
		a, err := NewAuthenticator(&config.AuthConfig{Mode: "jwt", JWTSecret: "secret"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := a.(*JWTAuthenticator); !ok {
			t.Fatalf("expected JWTAuthenticator, got %T", a)
		}
	})

	t.Run("apikey mode requires keys", func(t *testing.T) {
		_, err := NewAuthenticator(&config.AuthConfig{Mode: "apikey"})
		if err == nil {
			t.Fatal("expected error for missing api keys")
		}
	})

	t.Run("apikey mode with keys", func(t *testing.T) {
		a, err := NewAuthenticator(&config.AuthConfig{Mode: "apikey", APIKeys: map[string]config.APIKeyEntry{"k": {}}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := a.(*APIKeyAuthenticator); !ok {
			t.Fatalf("expected APIKeyAuthenticator, got %T", a)
		}
	})

	t.Run("unsupported mode", func(t *testing.T) {
		_, err := NewAuthenticator(&config.AuthConfig{Mode: "oauth2"})
		if err == nil {
			t.Fatal("expected error for unsupported mode")
		}
	})
}

func TestNoneAuthenticator(t *testing.T) {
	t.Run("default identity", func(t *testing.T) {
		a := NewNoneAuthenticator(nil)
		id, err := a.Authenticate(newRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.TenantID != tenant.DefaultID {
			t.Fatalf("tenant = %q", id.TenantID)
		}
		if id.Role != RoleOperator {
			t.Fatalf("role = %q", id.Role)
		}
	})

	t.Run("respects configured default role", func(t *testing.T) {
		a := NewNoneAuthenticator(&config.AuthConfig{DefaultRole: "auditor"})
		id, err := a.Authenticate(newRequest())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.Role != RoleAuditor {
			t.Fatalf("role = %q", id.Role)
		}
	})
}

func TestJWTAuthenticator(t *testing.T) {
	secret := "test-secret"
	a := NewJWTAuthenticator(&config.AuthConfig{JWTSecret: secret})

	t.Run("missing authorization header", func(t *testing.T) {
		_, err := a.Authenticate(newRequest())
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("non-bearer scheme", func(t *testing.T) {
		req := newRequest()
		req.Header.Set("Authorization", "Basic abc")
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("empty bearer token", func(t *testing.T) {
		req := newRequest()
		req.Header.Set("Authorization", "Bearer ")
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		req := newRequest()
		req.Header.Set("Authorization", "Bearer not-a-jwt")
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("bad signature", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "t1",
			"role":      "operator",
		})
		signed, _ := token.SignedString([]byte("wrong-secret"))
		req := newRequest()
		req.Header.Set("Authorization", "Bearer "+signed)
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "t1",
			"role":      "harness_designer",
		})
		signed, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		req := newRequest()
		req.Header.Set("Authorization", "Bearer "+signed)
		id, err := a.Authenticate(req)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if id.TenantID != tenant.ID("t1") {
			t.Fatalf("tenant = %q", id.TenantID)
		}
		if id.Role != RoleHarnessDesigner {
			t.Fatalf("role = %q", id.Role)
		}
	})

	t.Run("missing tenant_id claim", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"role": "operator",
		})
		signed, _ := token.SignedString([]byte(secret))
		req := newRequest()
		req.Header.Set("Authorization", "Bearer "+signed)
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("missing role claim", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "t1",
		})
		signed, _ := token.SignedString([]byte(secret))
		req := newRequest()
		req.Header.Set("Authorization", "Bearer "+signed)
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("issuer and audience validated", func(t *testing.T) {
		a2 := NewJWTAuthenticator(
			&config.AuthConfig{
				JWTSecret:   secret,
				JWTIssuer:   "hnsx",
				JWTAudience: "server",
			})
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"tenant_id": "t1",
			"role":      "operator",
			"iss":       "wrong",
			"aud":       "server",
		})
		signed, _ := token.SignedString([]byte(secret))
		req := newRequest()
		req.Header.Set("Authorization", "Bearer "+signed)
		_, err := a2.Authenticate(req)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})
}

func TestAPIKeyAuthenticator(t *testing.T) {
	a := NewAPIKeyAuthenticator(
		&config.AuthConfig{
			APIKeys: map[string]config.APIKeyEntry{
				"known-key": {
					TenantID: "tenant-42",
					Role:     "auditor",
				},
			},
		})

	t.Run("missing header", func(t *testing.T) {
		_, err := a.Authenticate(newRequest())
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		req := newRequest()
		req.Header.Set(apiKeyHeader, "unknown-key")
		_, err := a.Authenticate(req)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("known key", func(t *testing.T) {
		req := newRequest()
		req.Header.Set(apiKeyHeader, "known-key")
		id, err := a.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.TenantID != tenant.ID("tenant-42") {
			t.Fatalf("tenant = %q", id.TenantID)
		}
		if id.Role != RoleAuditor {
			t.Fatalf("role = %q", id.Role)
		}
	})

	t.Run("empty tenant falls back to default", func(t *testing.T) {
		a2 := NewAPIKeyAuthenticator(
			&config.AuthConfig{
				APIKeys: map[string]config.APIKeyEntry{
					"default-tenant-key": {},
				},
			})
		req := newRequest()
		req.Header.Set(apiKeyHeader, "default-tenant-key")
		id, err := a2.Authenticate(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.TenantID != tenant.DefaultID {
			t.Fatalf("tenant = %q", id.TenantID)
		}
		if id.Role != RoleOperator {
			t.Fatalf("role = %q", id.Role)
		}
	})
}

func TestContextAndHasRole(t *testing.T) {
	ctx := NewContext(t.Context(), &Identity{TenantID: tenant.DefaultID, Role: RoleOperator})
	if !HasRole(ctx, RoleOperator) {
		t.Fatal("expected HasRole to match operator")
	}
	if HasRole(ctx, RoleAuditor) {
		t.Fatal("expected HasRole to reject auditor")
	}
	if HasRole(t.Context(), RoleOperator) {
		t.Fatal("expected HasRole to reject missing identity")
	}
}

func TestNormalizeRole(t *testing.T) {
	cases := []struct {
		input    string
		expected Role
	}{
		{"platform_admin", RolePlatformAdmin},
		{"platform-admin", RolePlatformAdmin},
		{"admin", RolePlatformAdmin},
		{"harness_designer", RoleHarnessDesigner},
		{"harness-designer", RoleHarnessDesigner},
		{"designer", RoleHarnessDesigner},
		{"operator", RoleOperator},
		{"op", RoleOperator},
		{"auditor", RoleAuditor},
		{"audit", RoleAuditor},
		{"custom", Role("custom")},
	}
	for _, c := range cases {
		if got := normalizeRole(c.input); got != c.expected {
			t.Errorf("normalizeRole(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func newRequest() *http.Request {
	req, _ := http.NewRequest("GET", "/", nil)
	return req
}
