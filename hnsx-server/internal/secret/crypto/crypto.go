// Package crypto wraps the AES-GCM envelope used to encrypt Secret
// values at rest. The control plane derives a 32-byte key from the
// HNSX_SECRET_KEY environment variable (or a configured path); missing or
// malformed keys fail-fast at server start rather than silently
// downgrading to plaintext.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// Errors returned by the cipher.
var (
	ErrMissingKey = errors.New("crypto: HNSX_SECRET_KEY not set")
	ErrShortKey   = errors.New("crypto: HNSX_SECRET_KEY must be >=16 bytes")
	ErrDecrypt    = errors.New("crypto: decryption failed (wrong key or tampered ciphertext)")
)

// Cipher is an AES-GCM wrapper that the secret.Service holds as a
// dependency. Construct with New or NewFromKey; the zero value is unsafe.
type Cipher struct {
	gcm cipher.AEAD
}

// New constructs a Cipher by reading HNSX_SECRET_KEY from the environment.
// An empty key returns ErrMissingKey so the server fails-fast at start-up
// instead of quietly writing secrets as plaintext.
func New() (*Cipher, error) {
	key, err := EnvKeyFunc()
	if err != nil {
		return nil, err
	}
	return NewFromKey(key)
}

// EnvKeyFunc is the loader that reads HNSX_SECRET_KEY. Override in tests
// to inject a deterministic key without touching the environment.
var EnvKeyFunc = func() (string, error) {
	v := os.Getenv("HNSX_SECRET_KEY")
	if v == "" {
		return "", ErrMissingKey
	}
	return v, nil
}

// NewFromKey constructs a Cipher from an explicit key string. Strings
// shorter than 16 bytes are rejected (AES-128 minimum); the key is
// stretched to 32 bytes via SHA-256 to allow operators to pass a
// passphrase instead of a hex string.
func NewFromKey(key string) (*Cipher, error) {
	if key == "" {
		return nil, ErrMissingKey
	}
	if len(key) < 16 {
		return nil, ErrShortKey
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("crypto: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: cipher.NewGCM: %w", err)
	}
	return &Cipher{gcm: gcm}, nil
}

// Encrypt seals plaintext with a fresh nonce and returns the
// base64(nonce || ciphertext). Callers store the result verbatim; the
// nonce is not secret and must NOT be reused across writes.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if c == nil || c.gcm == nil {
		return "", errors.New("crypto: cipher not initialized")
	}
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := c.gcm.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt opens a base64(nonce || ciphertext) blob produced by Encrypt and
// returns the plaintext. A wrong key or any tampering surfaces as
// ErrDecrypt so callers do not echo garbled bytes back into the audit log.
func (c *Cipher) Decrypt(envelope string) (string, error) {
	if c == nil || c.gcm == nil {
		return "", errors.New("crypto: cipher not initialized")
	}
	raw, err := base64.StdEncoding.DecodeString(envelope)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) < ns {
		return "", ErrDecrypt
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}

// Fingerprint returns the last 4 chars of a base64(nonce || ciphertext)
// envelope — enough to verify a typed value without exposing the secret.
// Empty input returns "****".
func Fingerprint(envelope string) string {
	if envelope == "" {
		return "****"
	}
	if n := len(envelope); n > 4 {
		return "****" + envelope[n-4:]
	}
	return "****" + envelope
}

func loadKey() (string, error) {
	// Indirection so tests can stub the env without leaking the helper.
	return "", ErrMissingKey
}
