package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/secret/crypto"
	secmodel "github.com/hnsx-io/hnsx/server/internal/secret/model"
	secrepo "github.com/hnsx-io/hnsx/server/internal/secret/repository"
	secservice "github.com/hnsx-io/hnsx/server/internal/secret/service"
)

func newSecretTestServer(t *testing.T) (*Server, *secservice.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cipher, err := crypto.NewFromKey("test-passphrase-strong-enough")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	svc := secservice.NewService(secrepo.NewInMemoryRepository(), cipher)
	return &Server{SecretService: svc}, svc
}

func TestSecrets_CRUD_NeverReturnsPlaintext(t *testing.T) {
	s, _ := newSecretTestServer(t)
	const plaintext = "super-secret-api-key"

	// Create.
	body, _ := json.Marshal(map[string]any{
		"name":        "openai_api_key",
		"value":       plaintext,
		"description": "OpenAI prod key",
		"kind":        "api_key",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	s.CreateSecret(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), plaintext) {
		t.Fatalf("plaintext leaked in create response: %s", w.Body.String())
	}

	// List must show fingerprint, never value.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	s.ListSecrets(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), plaintext) {
		t.Fatalf("plaintext leaked in list response: %s", w.Body.String())
	}
	var listResp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != 1 || len(listResp.Items) != 1 {
		t.Fatalf("expected 1 secret, got total=%d items=%d", listResp.Total, len(listResp.Items))
	}
	if listResp.Items[0]["name"] != "openai_api_key" {
		t.Fatalf("item name wrong: %+v", listResp.Items[0])
	}
	if !strings.HasPrefix(listResp.Items[0]["fingerprint"].(string), "****") {
		t.Fatalf("fingerprint must be masked: %v", listResp.Items[0]["fingerprint"])
	}

	// Update (replace value).
	newPlaintext := "rotated-key"
	body, _ = json.Marshal(map[string]any{"value": newPlaintext, "kind": "api_key"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/openai_api_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "openai_api_key"}}
	s.UpdateSecret(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), newPlaintext) {
		t.Fatalf("plaintext leaked in update response")
	}

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/openai_api_key", nil)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "openai_api_key"}}
	s.DeleteSecret(c)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", w.Code)
	}
}

func TestSecrets_ResolveRoundTripsPlaintext(t *testing.T) {
	_, svc := newSecretTestServer(t)
	const plaintext = "embed-secret"
	if err := svc.Save(&secmodel.Secret{Name: "openai_api_key", PlainValue: plaintext}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := svc.Resolve("Bearer ${secret:openai_api_key}")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "Bearer embed-secret" {
		t.Fatalf("resolve got %q, want %q", got, "Bearer embed-secret")
	}
	// Missing name → placeholder must remain intact so caller surfaces error.
	_, err = svc.Resolve("${secret:missing}")
	if err != nil {
		t.Fatalf("resolve missing: %v", err)
	}
}

func TestSecrets_CreateMissingValue(t *testing.T) {
	s, _ := newSecretTestServer(t)
	body, _ := json.Marshal(map[string]any{"name": "k"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	s.CreateSecret(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSecrets_NilService(t *testing.T) {
	s := &Server{SecretService: nil}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	s.ListSecrets(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}
