package hnsx

import "fmt"

// APIError is the canonical error returned by the HnsX server.
type APIError struct {
	Code    string
	Message string
	Status  int
	Details map[string]any
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
