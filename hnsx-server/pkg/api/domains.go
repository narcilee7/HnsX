package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// ListDomains handles GET /api/v1/domains.
func (s *Server) ListDomains(w http.ResponseWriter, r *http.Request) {
	items := s.listDomainItems()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	out := make([]map[string]any, 0, len(items))
	for _, d := range items {
		out = append(out, map[string]any{
			"id":          d.ID,
			"version":     d.Version,
			"description": d.Description,
			"status":      "active",
			"created_at":  d.CreatedAt.UTC().Format(time.RFC3339),
			"updated_at":  d.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out,
		"total":  len(out),
		"limit":  len(out),
		"offset": 0,
	})
}

// GetDomain handles GET /api/v1/domains/{id}.
func (s *Server) GetDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.lookupDomain(id)
	if !ok {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          d.ID,
		"version":     d.Version,
		"description": d.Description,
		"harness":     d.Spec.Harness,
		"status":      "active",
		"created_at":  d.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  d.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

// RegisterDomain handles POST /api/v1/domains.
//
// Body can be either JSON (matches the canonical schema) or YAML
// (Content-Type: application/yaml or .yaml extension at /domains/import).
func (s *Server) RegisterDomain(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeDomainBody(r)
	if err != nil {
		writeError(w, r, NewValidation(err))
		return
	}

	if _, exists := s.lookupDomain(spec.ID); exists {
		writeError(w, r, &APIError{
			Code:    "DOMAIN_EXISTS",
			Message: "domain " + spec.ID + " already exists; use PUT to update",
			Details: map[string]any{"domain_id": spec.ID},
		})
		return
	}

	now := time.Now().UTC()
	d := &registeredDomain{
		ID:          spec.ID,
		Version:     spec.Version,
		Description: spec.Description,
		Spec:        spec,
		Harness:     spec.Harness,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.registerDomain(d)

	w.Header().Set("Location", "/api/v1/domains/"+spec.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         spec.ID,
		"version":    spec.Version,
		"created_at": now.Format(time.RFC3339),
	})
}

// UpdateDomain handles PUT /api/v1/domains/{id}.
func (s *Server) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, ok := s.lookupDomain(id)
	if !ok {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	spec, err := decodeDomainBody(r)
	if err != nil {
		writeError(w, r, NewValidation(err))
		return
	}
	if spec.ID != id {
		writeError(w, r, &APIError{
			Code:    "INVALID_REQUEST",
			Message: "domain id in body does not match URL",
		})
		return
	}

	existing.Version = spec.Version
	existing.Description = spec.Description
	existing.Spec = spec
	existing.Harness = spec.Harness
	existing.UpdatedAt = time.Now().UTC()
	s.registerDomain(existing) // re-register to refresh map

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         existing.ID,
		"version":    existing.Version,
		"updated_at": existing.UpdatedAt.Format(time.RFC3339),
	})
}

// DeleteDomain handles DELETE /api/v1/domains/{id}.
func (s *Server) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.mu.Lock()
	if _, ok := s.domains[id]; !ok {
		s.mu.Unlock()
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	delete(s.domains, id)
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// ListDomainVersions handles GET /api/v1/domains/{id}/versions.
//
// Phase 1 returns the currently registered version only — multi-version
// history is a future PR.
func (s *Server) ListDomainVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := s.lookupDomain(id)
	if !ok {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{{
			"version":    d.Version,
			"created_at": d.CreatedAt.UTC().Format(time.RFC3339),
			"is_current": true,
		}},
		"total": 1,
	})
}

// ValidateDomain handles POST /api/v1/domains/{id}/validate.
//
// Phase 1 re-validates the body against the v2 loader and returns the same
// summary as `hnsx validate`.
func (s *Server) ValidateDomain(w http.ResponseWriter, r *http.Request) {
	spec, err := decodeDomainBody(r)
	if err != nil {
		writeError(w, r, NewValidation(err))
		return
	}

	count := len(spec.Harness.Agents)
	steps := 0
	if spec.Harness.Session.Workflow != nil {
		steps = len(spec.Harness.Session.Workflow.Steps)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       true,
		"id":          spec.ID,
		"version":     spec.Version,
		"mode":        spec.Harness.Session.Mode,
		"agent_count": count,
		"step_count":  steps,
	})
}

// TriggerDomain handles POST /api/v1/domains/{id}/run.
//
// Equivalent to TriggerSession but the domain ID comes from the URL.
func (s *Server) TriggerDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Trigger map[string]any `json:"trigger"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		// Empty body is fine; default to {}.
		body.Trigger = map[string]any{}
	}
	s.triggerSession(w, r, id, body.Trigger)
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// decodeDomainBody parses either YAML or JSON body into a *spec.DomainSpec.
// It honours Content-Type ("application/yaml" -> yaml, default -> json) and
// also detects yaml format heuristically when the body starts with the YAML
// document marker "---".
func decodeDomainBody(r *http.Request) (*spec.DomainSpec, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var s spec.DomainSpec
	ct := r.Header.Get("Content-Type")
	if isYAMLContentType(ct) || looksLikeYAML(body) {
		if err := yaml.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	}
	if err := spec.Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}
