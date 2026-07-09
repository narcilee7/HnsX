package api

import (
	"errors"
	"testing"
)

func TestHTTPStatusFor(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"DOMAIN_NOT_FOUND", 404},
		{"SESSION_NOT_FOUND", 404},
		{"DOMAIN_EXISTS", 409},
		{"INVALID_REQUEST", 400},
		{"VALIDATION_FAILED", 400},
		{"UNAUTHENTICATED", 401},
		{"FORBIDDEN", 403},
		{"INTERNAL_ERROR", 500},
		{"ADAPTER_NOT_IMPLEMENTED", 422},
		{"???", 500},
	}
	for _, tc := range cases {
		if got := HTTPStatusFor(tc.code); got != tc.want {
			t.Errorf("HTTPStatusFor(%q) = %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestAPIError_ErrorMessage(t *testing.T) {
	ae := &APIError{Code: "FOO", Message: "bar"}
	if ae.Error() != "FOO: bar" {
		t.Errorf("got %q", ae.Error())
	}
	if (*APIError)(nil).Error() != "" {
		t.Fatal("nil APIError should have empty Error()")
	}
}

func TestAsAPIError(t *testing.T) {
	ae := NewDomainNotFound("x")
	if _, ok := AsAPIError(ae); !ok {
		t.Fatal("AsAPIError should detect APIError")
	}
	if _, ok := AsAPIError(errors.New("plain")); ok {
		t.Fatal("AsAPIError should not match plain error")
	}
	if _, ok := AsAPIError(nil); ok {
		t.Fatal("AsAPIError(nil) should be false")
	}
}

func TestStandardConstructors(t *testing.T) {
	cases := []struct {
		name string
		ae   *APIError
		code string
	}{
		{"NotFound", NewDomainNotFound("d"), "DOMAIN_NOT_FOUND"},
		{"Session", NewSessionNotFound("s"), "SESSION_NOT_FOUND"},
		{"Validation", NewValidation(errors.New("bad")), "VALIDATION_FAILED"},
		{"Invalid", NewInvalidRequest("nope"), "INVALID_REQUEST"},
		{"Internal", NewInternal(errors.New("boom")), "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		if tc.ae.Code != tc.code {
			t.Errorf("[%s] code = %q, want %q", tc.name, tc.ae.Code, tc.code)
		}
	}
}
