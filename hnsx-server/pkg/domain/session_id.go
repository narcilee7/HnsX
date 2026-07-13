// SessionID generation — moved from pkg/runtime/session_id.go in Phase 3.

package domain

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"strings"
	"time"
)

// NewSessionID generates a short, lexicographically sortable session id.
// Format: "<domain>-<unixnano-32base>-<rand-32base>". Random suffix avoids
// collisions when multiple sessions start in the same nanosecond.
func NewSessionID(domain string) string {
	var buf [8]byte
	ts := uint64(time.Now().UnixNano())
	binary.BigEndian.PutUint64(buf[:], ts)
	tsEnc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:])

	suf := make([]byte, 4)
	_, _ = rand.Read(suf)
	sufEnc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(suf)

	domainSlug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, domain)

	return domainSlug + "-" + strings.ToLower(tsEnc) + "-" + strings.ToLower(sufEnc)
}
