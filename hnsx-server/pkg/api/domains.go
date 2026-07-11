package api

import (
	"io"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	"github.com/hnsx-io/hnsx/server/pkg/local"
)

// ListDomains handles GET /api/v1/domains.
func (s *Server) ListDomains(c *gin.Context) {
	items := s.Queries.ListDomains(tenantFromGin(c))
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		out = append(out, map[string]any{
			"id":          d.ID,
			"version":     d.Version,
			"description": d.Description,
			"status":      d.Status,
			"created_at":  queries.FormatTimeValue(d.CreatedAt),
			"updated_at":  queries.FormatTimeValue(d.UpdatedAt),
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  len(out),
		"limit":  len(out),
		"offset": 0,
	})
}

// GetDomain handles GET /api/v1/domains/:id.
func (s *Server) GetDomain(c *gin.Context) {
	id := c.Param("id")
	item, d, ok := s.Queries.GetDomain(tenantFromGin(c), id)
	if !ok {
		writeError(c, NewDomainNotFound(id))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"id":          item.ID,
		"version":     item.Version,
		"description": item.Description,
		"harness":     d.Harness,
		"status":      item.Status,
		"created_at":  queries.FormatTimeValue(item.CreatedAt),
		"updated_at":  queries.FormatTimeValue(item.UpdatedAt),
	})
}

// GetDomainYAML handles GET /api/v1/domains/:id/yaml.
// Returns the canonical YAML representation of the registered domain spec.
// This is used by the console editor to avoid the protobuf/JSON shape mismatch.
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
	res, err := s.DomainCommands.Register(c.Request.Context(), tenantFromGin(c), c.Request.Body, c.ContentType())
	if err != nil {
		if err == commands.ErrDomainExists {
			writeError(c, &APIError{
				Code:    "DOMAIN_EXISTS",
				Message: err.Error(),
				Details: map[string]any{"domain_id": res.Domain.ID},
			})
			return
		}
		writeError(c, NewValidation(err))
		return
	}

	_ = s.LoadDomainPolicy(c.Request.Context(), res.Domain.ID)

	c.Header("Location", commands.BuildDomainLocation(res.Domain.ID))
	writeJSON(c, http.StatusCreated, map[string]any{
		"id":         res.Domain.ID,
		"version":    res.Domain.Version,
		"created_at": queries.FormatTimeValue(res.CreatedAt),
	})
}

// UpdateDomain handles PUT /api/v1/domains/:id.
func (s *Server) UpdateDomain(c *gin.Context) {
	id := c.Param("id")
	updated, err := s.DomainCommands.Update(c.Request.Context(), tenantFromGin(c), id, c.Request.Body, c.ContentType())
	if err != nil {
		switch err {
		case commands.ErrDomainNotFound:
			writeError(c, NewDomainNotFound(id))
		case commands.ErrIDMismatch:
			writeError(c, &APIError{
				Code:    "INVALID_REQUEST",
				Message: "domain id in body does not match URL",
			})
		default:
			writeError(c, NewValidation(err))
		}
		return
	}

	_ = s.LoadDomainPolicy(c.Request.Context(), updated.ID)

	writeJSON(c, http.StatusOK, map[string]any{
		"id":         updated.ID,
		"version":    updated.Version,
		"updated_at": updated.UpdatedAt,
	})
}

// DeleteDomain handles DELETE /api/v1/domains/:id.
func (s *Server) DeleteDomain(c *gin.Context) {
	id := c.Param("id")
	if err := s.DomainCommands.Delete(c.Request.Context(), tenantFromGin(c), id); err != nil {
		writeError(c, NewDomainNotFound(id))
		return
	}
	c.Status(http.StatusNoContent)
}

// ListDomainVersions handles GET /api/v1/domains/:id/versions.
func (s *Server) ListDomainVersions(c *gin.Context) {
	id := c.Param("id")
	item, _, ok := s.Queries.GetDomain(tenantFromGin(c), id)
	if !ok {
		writeError(c, NewDomainNotFound(id))
		return
	}
	versions, ok := s.Queries.ListDomainVersions(tenantFromGin(c), id)
	if !ok {
		writeError(c, NewDomainNotFound(id))
		return
	}

	out := make([]map[string]any, 0, len(versions))
	for _, v := range versions {
		out = append(out, map[string]any{
			"version":    v.Version,
			"created_at": queries.FormatTimeValue(v.CreatedAt),
			"is_current": v.Version == item.Version,
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  len(out),
		"limit":  len(out),
		"offset": 0,
	})
}

// GetDomainVersion handles GET /api/v1/domains/:id/versions/:version.
func (s *Server) GetDomainVersion(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")
	d, ok := s.Queries.GetDomainVersion(tenantFromGin(c), id, version)
	if !ok {
		writeError(c, NewDomainNotFound(id))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"id":          d.ID,
		"version":     d.Version,
		"description": d.Description,
		"harness":     d.Harness,
		"status":      "active",
		"created_at":  d.CreatedAt,
		"updated_at":  d.UpdatedAt,
	})
}

// ValidateDomain handles POST /api/v1/domains/:id/validate.
func (s *Server) ValidateDomain(c *gin.Context) {
	summary, err := local.ValidateDomain(c.Request.Body, c.ContentType())
	if err != nil {
		writeError(c, NewValidation(err))
		return
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"valid":       summary.Valid,
		"id":          summary.ID,
		"version":     summary.Version,
		"mode":        summary.Mode,
		"agent_count": summary.AgentCount,
		"step_count":  summary.StepCount,
		"tenant_id":   tenantFromGin(c),
	})
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
