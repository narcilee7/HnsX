package api

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
)

// ListDomains handles GET /api/v1/domains.
func (s *Server) ListDomains(c *gin.Context) {
	out, err := s.Handlers.ListDomains(c.Request.Context(), handler.ListDomainsInput{
		TenantID: tenantFromGin(c),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Domains)
}

// GetDomain handles GET /api/v1/domains/:id.
func (s *Server) GetDomain(c *gin.Context) {
	out, err := s.Handlers.GetDomain(c.Request.Context(), handler.GetDomainInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Domain)
}

// GetDomainYAML handles GET /api/v1/domains/:id/yaml.
// Returns the canonical YAML representation of the registered domain spec.
func (s *Server) GetDomainYAML(c *gin.Context) {
	id := c.Param("id")
	_, d, ok := s.Queries.GetDomain(tenantFromGin(c), id)
	if !ok {
		writeError(c, NewDomainNotFound(id))
		return
	}
	body, err := yaml.Marshal(d.Spec)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	c.Header("Content-Type", "application/yaml")
	c.String(http.StatusOK, string(body))
}

// RegisterDomain handles POST /api/v1/domains.
func (s *Server) RegisterDomain(c *gin.Context) {
	out, err := s.Handlers.RegisterDomain(c.Request.Context(), handler.RegisterDomainInput{
		TenantID:    tenantFromGin(c),
		Body:        c.Request.Body,
		ContentType: c.ContentType(),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}

	_ = s.LoadDomainPolicy(c.Request.Context(), out.Domain.ID)

	c.Header("Location", commands.BuildDomainLocation(out.Domain.ID))
	writeJSON(c, http.StatusCreated, out.Domain)
}

// UpdateDomain handles PUT /api/v1/domains/:id.
func (s *Server) UpdateDomain(c *gin.Context) {
	id := c.Param("id")
	out, err := s.Handlers.UpdateDomain(c.Request.Context(), handler.UpdateDomainInput{
		TenantID:    tenantFromGin(c),
		ID:          id,
		Body:        c.Request.Body,
		ContentType: c.ContentType(),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}

	_ = s.LoadDomainPolicy(c.Request.Context(), out.Domain.ID)

	writeJSON(c, http.StatusOK, out.Domain)
}

// DeleteDomain handles DELETE /api/v1/domains/:id.
func (s *Server) DeleteDomain(c *gin.Context) {
	err := s.Handlers.DeleteDomain(c.Request.Context(), handler.DeleteDomainInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// ListDomainVersions handles GET /api/v1/domains/:id/versions.
func (s *Server) ListDomainVersions(c *gin.Context) {
	out, err := s.Handlers.ListDomainVersions(c.Request.Context(), handler.ListDomainVersionsInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Versions)
}

// GetDomainVersion handles GET /api/v1/domains/:id/versions/:version.
func (s *Server) GetDomainVersion(c *gin.Context) {
	out, err := s.Handlers.GetDomainVersion(c.Request.Context(), handler.GetDomainVersionInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
		Version:  c.Param("version"),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Domain)
}

// GetDomainSchema handles GET /api/v1/domains/:id/schema.
func (s *Server) GetDomainSchema(c *gin.Context) {
	out, err := s.Handlers.GetDomainSchema(c.Request.Context(), handler.GetDomainSchemaInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, mapDomainError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Schema)
}

// ValidateDomain handles POST /api/v1/domains/:id/validate.
func (s *Server) ValidateDomain(c *gin.Context) {
	out, err := s.Handlers.ValidateDomain(c.Request.Context(), handler.ValidateDomainInput{
		Body:        c.Request.Body,
		ContentType: c.ContentType(),
	})
	if err != nil {
		writeError(c, NewValidation(err))
		return
	}
	out.Summary.TenantID = string(tenantFromGin(c))
	writeJSON(c, http.StatusOK, out.Summary)
}

// TriggerDomain handles POST /api/v1/domains/:id/run.
func (s *Server) TriggerDomain(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Trigger map[string]any `json:"trigger"`
	}
	if err := decodeJSONBody(c, &body); err != nil {
		body.Trigger = map[string]any{}
	}
	s.triggerSession(c, tenantFromGin(c), id, body.Trigger)
}

// readDomainBody is a small helper kept in this file for handler use.
func readDomainBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// mapDomainError maps handler/domain errors to canonical APIError values.
func mapDomainError(err error) *APIError {
	if err == nil {
		return nil
	}
	if handler.IsDomainNotFound(err) {
		// The actual ID is not preserved here; callers that need it construct
		// the error themselves before calling writeError.
		return NewDomainNotFound("")
	}
	if handler.IsDomainExists(err) {
		return &APIError{Code: "DOMAIN_EXISTS", Message: err.Error()}
	}
	if handler.IsIDMismatch(err) {
		return &APIError{Code: "INVALID_REQUEST", Message: "domain id in body does not match URL"}
	}
	if handler.IsInvalidSpec(err) {
		return NewValidation(err)
	}
	return NewInternal(err)
}
