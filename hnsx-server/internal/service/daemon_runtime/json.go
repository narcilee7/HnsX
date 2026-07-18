package daemon_runtime

import "encoding/json"

// defaultJSONUnmarshal is the production json decoder, separated out
// from the test seam in service.go so tests can swap it cleanly.
var defaultJSONUnmarshal = json.Unmarshal