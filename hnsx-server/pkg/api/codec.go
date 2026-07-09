package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/hnsx-io/hnsx/core/domain"
)

// loadedDomain is an alias used by decodeDomainBody. It deliberately mirrors
// domain.DomainSpec so the loader's Validate function can run unchanged.
type loadedDomain = domain.DomainSpec

// decodeJSONBody is a small wrapper that reads the request body and decodes
// it into v as JSON. Empty body is treated as empty-object success.
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

// isYAMLContentType returns true when the request Content-Type indicates a
// YAML payload (application/yaml, text/yaml, or any *+yaml suffix).
func isYAMLContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mediatype, _, _ := mime.ParseMediaType(ct)
	mediatype = strings.ToLower(mediatype)
	if strings.HasSuffix(mediatype, "+yaml") || strings.HasSuffix(mediatype, "+yml") {
		return true
	}
	return mediatype == "application/yaml" || mediatype == "text/yaml" ||
		mediatype == "application/x-yaml"
}

// looksLikeYAML is a heuristic for bodies that omit Content-Type but start
// with the canonical YAML document marker.
func looksLikeYAML(body []byte) bool {
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return bytes.HasPrefix(trimmed, []byte("---"))
}
