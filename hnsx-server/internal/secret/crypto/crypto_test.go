package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	c, err := NewFromKey("test-passphrase-strong-enough")
	if err != nil {
		t.Fatal(err)
	}
	env, err := c.Encrypt("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(env, "hello world") {
		t.Fatal("plaintext leaked")
	}
	got, err := c.Decrypt(env)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestEmptyKeyFailsFast(t *testing.T) {
	if _, err := NewFromKey(""); err != ErrMissingKey {
		t.Fatalf("got %v, want ErrMissingKey", err)
	}
}

func TestShortKeyRejected(t *testing.T) {
	if _, err := NewFromKey("short"); err != ErrShortKey {
		t.Fatalf("got %v, want ErrShortKey", err)
	}
}

func TestDecryptTamperedFails(t *testing.T) {
	c, _ := NewFromKey("test-passphrase-strong-enough")
	env, _ := c.Encrypt("hello world")
	// Decode, flip a byte in the ciphertext region, then re-encode so the
	// input remains valid base64 and the failure comes from AEAD verification.
	raw, err := base64.StdEncoding.DecodeString(env)
	if err != nil {
		t.Fatalf("decode own ciphertext: %v", err)
	}
	ns := c.gcm.NonceSize()
	if len(raw) <= ns {
		t.Fatal("ciphertext too short")
	}
	raw[ns] ^= 0x01
	bad := base64.StdEncoding.EncodeToString(raw)
	if _, err := c.Decrypt(bad); err != ErrDecrypt {
		t.Fatalf("got %v, want ErrDecrypt", err)
	}
}

func TestFingerprint(t *testing.T) {
	if got := Fingerprint(""); got != "****" {
		t.Fatalf("got %q", got)
	}
	if got := Fingerprint("abcdefgh"); got != "****efgh" {
		t.Fatalf("got %q", got)
	}
}
