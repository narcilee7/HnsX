package multica_adapter

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// writeJSON serializes body and writes it with the given status code.
func writeJSON(c *gin.Context, status int, body any) {
	c.JSON(status, body)
}

// rawEmptyObject returns a JSON-encoded "{}" so Multica's parser sees a JSON
// object instead of null for unset JSONB columns.
func rawEmptyObject() json.RawMessage {
	return json.RawMessage(`{}`)
}

// rawEmptyArray returns a JSON-encoded "[]".
func rawEmptyArray() json.RawMessage {
	return json.RawMessage(`[]`)
}

// tenantFromGin reads the HnsX tenant id from gin context (set by the
// tenantMiddleware on the parent gin engine).
func tenantFromGin(c *gin.Context) tenant.ID {
	if v, ok := c.Get("hnsx-tenant-id"); ok {
		if id, ok := v.(tenant.ID); ok {
			return id
		}
	}
	return tenant.DefaultID
}

// errorJSON writes an error envelope matching Multica's error shape.
func errorJSON(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": msg,
		},
	})
}

// notImplemented is a 501 helper for endpoints the adapter hasn't wired yet.
func notImplemented(c *gin.Context, name string) {
	errorJSON(c, http.StatusNotImplemented, "NOT_IMPLEMENTED", name+" not implemented in multica_adapter yet")
}
