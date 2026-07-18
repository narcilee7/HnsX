package agentruntime

import "os"

// osEnviron is a tiny indirection so tests can override the environment
// passed to spawned subprocesses without leaking the helper across files.
var osEnviron = os.Environ