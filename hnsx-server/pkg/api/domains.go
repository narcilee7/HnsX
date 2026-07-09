package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	"github.com/hnsx-io/hnsx/server/internal/app/queries"
)

// ListDomains handles GET /api/v1/domains.
func (s *Server) ListDomains(w http.ResponseWriter, r *http.Request) {
	items := queries.ListDomains(s.AppState)
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
	item, d, ok := queries.GetDomain(s.AppState, id)
	if !ok {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          item.ID,
		"version":     item.Version,
		"description": item.Description,
		"harness":     d.Harness,
		"status":      item.Status,
		"created_at":  queries.FormatTimeValue(item.CreatedAt),
		"updated_at":  queries.FormatTimeValue(item.UpdatedAt),
	})
}

// RegisterDomain handles POST /api/v1/domains.
func (s *Server) RegisterDomain(w http.ResponseWriter, r *http.Request) {
	res, err := commands.RegisterDomain(s.AppState, r.Body, r.Header.Get("Content-Type"))
	if err != nil {
		if err == commands.ErrDomainExists {
			writeError(w, r, &APIError{
				Code:    "DOMAIN_EXISTS",
				Message: err.Error(),
				Details: map[string]any{"domain_id": res.Domain.ID},
			})
			return
		}
		writeError(w, r, NewValidation(err))
		return
	}

	_ = s.LoadDomainPolicy(res.Domain.ID)

	w.Header().Set("Location", commands.BuildDomainLocation(res.Domain.ID))
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         res.Domain.ID,
		"version":    res.Domain.Version,
		"created_at": queries.FormatTimeValue(res.CreatedAt),
	})
}

// UpdateDomain handles PUT /api/v1/domains/{id}.
func (s *Server) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	updated, err := commands.UpdateDomain(s.AppState, id, r.Body, r.Header.Get("Content-Type"))
	if err != nil {
		switch err {
		case commands.ErrDomainNotFound:
			writeError(w, r, NewDomainNotFound(id))
		case commands.ErrIDMismatch:
			writeError(w, r, &APIError{
				Code:    "INVALID_REQUEST",
				Message: "domain id in body does not match URL",
			})
		default:
			writeError(w, r, NewValidation(err))
		}
		return
	}

	_ = s.LoadDomainPolicy(updated.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         updated.ID,
		"version":    updated.Version,
		"updated_at": queries.FormatTimeValue(updated.UpdatedAt),
	})
}

// DeleteDomain handles DELETE /api/v1/domains/{id}.
func (s *Server) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := commands.DeleteDomain(s.AppState, id); err != nil {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListDomainVersions handles GET /api/v1/domains/{id}/versions.
//
// Phase 1 returns the currently registered version only — multi-version
// history is a future PR.
func (s *Server) ListDomainVersions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, d, ok := queries.GetDomain(s.AppState, id)
	if !ok {
		writeError(w, r, NewDomainNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{{
			"version":    d.Version,
			"created_at": queries.FormatTimeValue(d.CreatedAt),
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
	summary, err := commands.ValidateDomain(r.Body, r.Header.Get("Content-Type"))
	if err != nil {
		writeError(w, r, NewValidation(err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":       summary.Valid,
		"id":          summary.ID,
		"version":     summary.Version,
		"mode":        summary.Mode,
		"agent_count": summary.AgentCount,
		"step_count":  summary.StepCount,
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

// decodeJSONBody is a small wrapper kept in this file for handler use.
func decodeJSONBody(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return json.Unmarshal(body, v)
}

