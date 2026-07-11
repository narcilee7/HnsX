package cli

import (
	"testing"
)

func TestWritableHome(t *testing.T) {
	t.Setenv("HOME", "/Users/example")
	if !writableHome("/Users/example/.local/bin/hnsx") {
		t.Fatal("path under HOME should be writable")
	}
	if writableHome("/usr/local/bin/hnsx") {
		t.Fatal("path outside HOME should not be writable")
	}
	if writableHome("relative/path") {
		t.Fatal("relative path should not be writable")
	}
}

func TestMatchAssetName(t *testing.T) {
	cases := map[string]struct {
		name, want string
		ok         bool
	}{
		"hnsx_0.8.0_darwin_arm64":     {"hnsx_0.8.0_darwin_arm64.tar.gz", "hnsx_darwin_arm64", true},
		"hnsx_0.8.0_linux_amd64.tar":  {"hnsx_0.8.0_linux_amd64.tar.gz", "hnsx_linux_amd64", true},
		"wrong-prefix":                {"other_0.8.0_darwin_arm64.tar.gz", "hnsx_darwin_arm64", false},
		"too-short":                   {"hnsx_darwin", "hnsx_darwin_arm64", false},
		"empty":                       {"", "hnsx_darwin_arm64", false},
	}
	for label, c := range cases {
		got := matchAssetName(c.name, c.want)
		if got != c.ok {
			t.Errorf("%s: matchAssetName(%q, %q) = %v, want %v", label, c.name, c.want, got, c.ok)
		}
	}
}

func TestPickAsset(t *testing.T) {
	r := &releaseInfo{
		TagName: "v0.8.0",
		Assets: []releaseAsset{
			{Name: "hnsx_0.8.0_darwin_arm64.tar.gz", URL: "darwin"},
			{Name: "hnsx_0.8.0_linux_amd64.tar.gz", URL: "linux"},
			{Name: "checksums.txt", URL: "c"},
		},
	}
	got := pickAsset(r, "darwin", "arm64")
	if got == nil || got.URL != "darwin" {
		t.Fatalf("expected darwin asset, got %+v", got)
	}
	if pickAsset(r, "windows", "amd64") != nil {
		t.Fatal("expected nil for unsupported OS")
	}
}