// Package api hosts the HTTP/REST API and SSE handlers for hnsx-server.
//
// Errors emitted by handlers MUST conform to the APIError envelope so the
// console (and any AI agent consumer) can rely on a stable shape:
//
//	{
//	  "code":    "DOMAIN_NOT_FOUND",
//	  "message": "domain 'customer-service' not found",
//	  "details": { ... }   // optional, machine-readable context
//	}
//
// The mapping from error -> HTTP status is centralized in HTTPStatusFor so
// errors propagate consistently across all resource groups.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// APIError is the canonical error payload returned by every handler.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Error implements error so APIError can be returned from helpers that wrap
// service-layer errors.
func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// AsAPIError tries to interpret err as an *APIError. Returns false if err is
// nil or no APIError is in the chain.
func AsAPIError(err error) (*APIError, bool) {
	if err == nil {
		return nil, false
	}
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// writeError serializes err onto w with the appropriate HTTP status. If err
// is not an APIError, a generic INTERNAL_ERROR is returned with status 500.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ae, ok := AsAPIError(err)
	if !ok {
		ae = &APIError{
			Code:    "INTERNAL_ERROR",
			Message: err.Error(),
		}
	}
	status := HTTPStatusFor(ae.Code)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ae)
}

// writeJSON is a tiny convenience that handles Content-Type and encoding.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// HTTPStatusFor maps a stable code to an HTTP status. Codes not in the table
// default to 500 / INTERNAL_ERROR.
func HTTPStatusFor(code string) int {
	switch code {
	case "INVALID_REQUEST", "VALIDATION_FAILED", "MISSING_PARAMETER",
		"INVALID_YAML", "INVALID_JSON", "INVALID_MODE":
		return http.StatusBadRequest
	case "UNAUTHENTICATED":
		return http.StatusUnauthorized
	case "FORBIDDEN":
		return http.StatusForbidden
	case "DOMAIN_NOT_FOUND", "SESSION_NOT_FOUND", "EVAL_SET_NOT_FOUND",
		"EVAL_RUN_NOT_FOUND", "TRACE_NOT_FOUND", "APPROVAL_NOT_FOUND",
		"SECRET_NOT_FOUND", "POLICY_NOT_FOUND", "RUNTIME_NOT_FOUND":
		return http.StatusNotFound
	case "DOMAIN_EXISTS", "VERSION_EXISTS", "EVAL_SET_EXISTS":
		return http.StatusConflict
	case "ADAPTER_NOT_IMPLEMENTED", "MODE_NOT_IMPLEMENTED":
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// ----------------------------------------------------------------------------
// Standard constructors so call sites stay short.
// ----------------------------------------------------------------------------

func NewDomainNotFound(id string) *APIError {
	return &APIError{
		Code:    "DOMAIN_NOT_FOUND",
		Message: "domain " + id + " not found",
		Details: map[string]any{"domain_id": id},
	}
}

func NewSessionNotFound(id string) *APIError {
	return &APIError{
		Code:    "SESSION_NOT_FOUND",
		Message: "session " + id + " not found",
		Details: map[string]any{"session_id": id},
	}
}

func NewValidation(err error) *APIError {
	return &APIError{
		Code:    "VALIDATION_FAILED",
		Message: err.Error(),
	}
}

func NewInvalidRequest(reason string) *APIError {
	return &APIError{
		Code:    "INVALID_REQUEST",
		Message: reason,
	}
}

func NewInternal(cause error) *APIError {
	return &APIError{
		Code:    "INTERNAL_ERROR",
		Message: cause.Error(),
	}
}
