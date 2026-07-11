package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListTemplates_EmptyWhenPathUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{TemplatesIndexPath: ""}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	s.ListTemplates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("expected empty list, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

func TestListTemplates_LoadsIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	path := filepath.Join(dir, "index.yaml")
	data := []byte(`version: "1.0"
templates:
  - id: demo
    name: Demo
    description: A demo template.
    tags: [demo, test]
    source: example-domains/demo/domain.yaml
    variables:
      - name: company
        default: Acme
    requirements:
      providers: [anthropic]
      min_workers: 1
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	s := &Server{TemplatesIndexPath: path}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	s.ListTemplates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 template, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	it := resp.Items[0]
	if it["id"] != "demo" {
		t.Fatalf("id = %v", it["id"])
	}
	if it["name"] != "Demo" {
		t.Fatalf("name = %v", it["name"])
	}
	tags, _ := it["tags"].([]any)
	if len(tags) != 2 {
		t.Fatalf("tags = %v", tags)
	}
}

func TestListTemplates_TagFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	path := filepath.Join(dir, "index.yaml")
	data := []byte(`version: "1.0"
templates:
  - id: a
    name: A
    description: Alpha
    tags: [alpha]
    source: a/domain.yaml
    variables: []
    requirements: {}
  - id: b
    name: B
    description: Beta
    tags: [beta]
    source: b/domain.yaml
    variables: []
    requirements: {}
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	s := &Server{TemplatesIndexPath: path}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/templates?tag=beta", nil)
	s.ListTemplates(c)

	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0]["id"] != "b" {
		t.Fatalf("expected only beta template, got %+v", resp.Items)
	}
}

func TestListTemplates_MissingFileReturnsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{TemplatesIndexPath: filepath.Join(t.TempDir(), "missing.yaml")}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	s.ListTemplates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 || len(resp.Items) != 0 {
		t.Fatalf("expected empty list for missing file, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}
