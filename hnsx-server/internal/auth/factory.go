package auth

import (
	"fmt"

	"github.com/hnsx-io/hnsx/server/internal/config"
)

// NewAuthenticator selects an authenticator based on cfg.Mode.
func NewAuthenticator(cfg *config.AuthConfig) (Authenticator, error) {
	if cfg == nil {
		cfg = &config.AuthConfig{Mode: "none"}
	}
	switch cfg.Mode {
	case "", "none":
		return NewNoneAuthenticator(cfg), nil
	case "jwt":
		if cfg.JWTSecret == "" {
			return nil, fmt.Errorf("auth mode jwt requires HNSX_AUTH_JWT_SECRET")
		}
		return NewJWTAuthenticator(cfg), nil
	case "apikey":
		if len(cfg.APIKeys) == 0 {
			return nil, fmt.Errorf("auth mode apikey requires at least one API key")
		}
		return NewAPIKeyAuthenticator(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported auth mode %q", cfg.Mode)
	}
}
