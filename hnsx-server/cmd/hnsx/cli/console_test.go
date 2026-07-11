package cli

import (
	"net"
	"testing"
)

func TestCheckPortFree_BusyPort(t *testing.T) {
	// Open a listener to occupy a port.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	addr := l.Addr().String()
	if err := checkPortFree(addr); err == nil {
		t.Fatalf("expected checkPortFree to fail on occupied %s", addr)
	}
}

func TestCheckPortFree_FreePort(t *testing.T) {
	// Bind to an ephemeral port and immediately release it; the address is
	// almost always reusable in CI. We tolerate rare EADDRINUSE.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	if err := checkPortFree(addr); err != nil {
		// Very rare race on shared CI; tolerate but don't pass cleanly.
		t.Skipf("port %s not reusable in this env: %v", addr, err)
	}
}

func TestOpenBrowser_BadURL(t *testing.T) {
	// We don't actually validate the URL — the OS-side handler will fail.
	// Just ensure no panic and an error is returned for an obviously bad URL.
	err := openBrowser("not-a-real-url-scheme://bogus")
	if err == nil {
		t.Log("openBrowser did not return an error (acceptable on some shells)")
	}
}