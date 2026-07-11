package auth

import (
	"net/http"

	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// NoneAuthenticator always returns the default tenant/role identity.
type NoneAuthenticator struct {
	cfg *config.AuthConfig
}

// NewNoneAuthenticator constructs a none-mode authenticator.
func NewNoneAuthenticator(cfg *config.AuthConfig) *NoneAuthenticator {
	if cfg == nil {
		cfg = &config.AuthConfig{}
	}
	return &NoneAuthenticator{cfg: cfg}
}

// Authenticate implements Authenticator.
func (a *NoneAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
	return &Identity{
		TenantID: tenant.DefaultID,
		Role:     defaultRole(a.cfg),
	}, nil
}
