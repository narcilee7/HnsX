// Package auth provides authentication, tenant mapping, and RBAC context
// propagation for incoming HTTP requests.
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// Role is a named authorization role.
type Role string

// Well-known roles.
const (
	RolePlatformAdmin  Role = "platform_admin"
	RoleHarnessDesigner Role = "harness_designer"
	RoleOperator       Role = "operator"
	RoleAuditor        Role = "auditor"
	RoleNone           Role = "none"
)

// Identity is the authenticated caller identity.
type Identity struct {
	TenantID tenant.ID
	Role     Role
}

// Authenticator extracts an Identity from an HTTP request.
type Authenticator interface {
	Authenticate(r *http.Request) (*Identity, error)
}

// ErrUnauthorized indicates missing or malformed credentials.
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden indicates valid credentials that are not allowed to perform an action.
var ErrForbidden = errors.New("forbidden")

type contextKey struct{}

// NewContext returns a context carrying the authenticated identity.
func NewContext(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the identity from the context, or nil if none is present.
func FromContext(ctx context.Context) *Identity {
	if id, ok := ctx.Value(contextKey{}).(*Identity); ok {
		return id
	}
	return nil
}

// HasRole reports whether the identity in ctx has any of the supplied roles.
// If no identity is present, the call returns false.
func HasRole(ctx context.Context, roles ...Role) bool {
	id := FromContext(ctx)
	if id == nil {
		return false
	}
	for _, r := range roles {
		if id.Role == r {
			return true
		}
	}
	return false
}

// normalizeRole maps configured role strings to the canonical Role type.
func normalizeRole(s string) Role {
	switch s {
	case string(RolePlatformAdmin), "platform-admin", "admin":
		return RolePlatformAdmin
	case string(RoleHarnessDesigner), "harness-designer", "designer":
		return RoleHarnessDesigner
	case string(RoleOperator), "op":
		return RoleOperator
	case string(RoleAuditor), "audit":
		return RoleAuditor
	default:
		return Role(s)
	}
}

func defaultRole(cfg *config.AuthConfig) Role {
	if cfg.DefaultRole == "" {
		return RoleOperator
	}
	return normalizeRole(cfg.DefaultRole)
}
