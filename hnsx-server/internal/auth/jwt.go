package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/hnsx-io/hnsx/server/internal/config"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// JWTAuthenticator verifies Bearer tokens signed with HS256.
type JWTAuthenticator struct {
	cfg *config.AuthConfig
}

// NewJWTAuthenticator constructs a JWT authenticator.
func NewJWTAuthenticator(cfg *config.AuthConfig) *JWTAuthenticator {
	if cfg == nil {
		cfg = &config.AuthConfig{}
	}
	return &JWTAuthenticator{cfg: cfg}
}

// Authenticate implements Authenticator.
func (a *JWTAuthenticator) Authenticate(r *http.Request) (*Identity, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, fmt.Errorf("%w: missing Authorization header", ErrUnauthorized)
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return nil, fmt.Errorf("%w: Authorization must be Bearer token", ErrUnauthorized)
	}
	tokenString := strings.TrimPrefix(h, prefix)
	if tokenString == "" {
		return nil, fmt.Errorf("%w: empty bearer token", ErrUnauthorized)
	}

	keyFunc := func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrForbidden, token.Header["alg"])
		}
		return []byte(a.cfg.JWTSecret), nil
	}

	parserOpts := []jwt.ParserOption{}
	if a.cfg.JWTIssuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(a.cfg.JWTIssuer))
	}
	if a.cfg.JWTAudience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(a.cfg.JWTAudience))
	}
	parser := jwt.NewParser(parserOpts...)
	token, err := parser.Parse(tokenString, keyFunc)
	if err != nil {
		if isAuthenticationError(err) {
			return nil, fmt.Errorf("%w: %v", ErrUnauthorized, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrForbidden, err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("%w: invalid token", ErrForbidden)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: invalid token claims", ErrForbidden)
	}

	identity := &Identity{
		TenantID: tenant.DefaultID,
		Role:     RoleOperator,
	}

	if v, ok := claims["tenant_id"].(string); ok && v != "" {
		identity.TenantID = tenant.ID(v)
	} else {
		return nil, fmt.Errorf("%w: missing or invalid tenant_id claim", ErrForbidden)
	}

	if v, ok := claims["role"].(string); ok && v != "" {
		identity.Role = normalizeRole(v)
	} else {
		return nil, fmt.Errorf("%w: missing or invalid role claim", ErrForbidden)
	}

	return identity, nil
}

func isAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	// Malformed or entirely missing structural parts are treated as 401.
	// Valid structure but bad signature/claims are 403.
	s := err.Error()
	return strings.Contains(s, "token contains an invalid number of segments") ||
		strings.Contains(s, "token is malformed")
}
