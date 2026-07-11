#!/usr/bin/env bash
# scripts/install.sh — canonical installer for the HnsX CLI.
#
# Resolves the latest release tag via the GitHub API, picks the matching asset
# for the host OS/arch, downloads it to a temp file, verifies the SHA-256
# checksum against the release's checksums.txt, extracts the binaries, and
# installs them into $HOME/.local/bin (or $HNSX_INSTALL_DIR).
#
# Usage:
#   curl -sSL hnsx.dev/install.sh | sh
#   HNSX_INSTALL_DIR=/usr/local/bin ./scripts/install.sh

set -euo pipefail

REPO="hnsx-io/hnsx"
BIN_NAME="hnsx"
SERVER_BIN_NAME="hnsx-server"
INSTALL_DIR="${HNSX_INSTALL_DIR:-$HOME/.local/bin}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "✗ missing: $1" >&2; exit 1; }; }

need curl
need tar
need shasum

# Detect OS/arch.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "✗ unsupported arch: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) echo "✗ unsupported OS: $OS" >&2; exit 1 ;;
esac

echo "→ resolving latest $REPO release ..."
tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
       | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
if [[ -z "$tag" ]]; then
  echo "✗ could not determine latest release" >&2
  exit 1
fi
echo "→ latest tag: $tag"

asset="hnsx_${OS}_${ARCH}.tar.gz"
url="https://github.com/$REPO/releases/download/$tag/$asset"
checksums_url="https://github.com/$REPO/releases/download/$tag/checksums.txt"
echo "→ downloading $url"

tmpdir="$(mktemp -d -t hnsx-install.XXXXXX)"
trap 'rm -rf "$tmpdir"' EXIT
curl -fsSL "$url" -o "$tmpdir/$asset"

echo "→ verifying checksum ..."
curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt"
cd "$tmpdir"
expected="$(grep -E "^([0-9a-f]+)  ${asset}$" checksums.txt | awk '{print $1}')"
if [[ -z "$expected" ]]; then
  echo "✗ could not find checksum for $asset" >&2
  exit 1
fi
actual="$(shasum -a 256 "$asset" | awk '{print $1}')"
if [[ "$expected" != "$actual" ]]; then
  echo "✗ checksum mismatch for $asset" >&2
  echo "  expected: $expected" >&2
  echo "  actual:   $actual" >&2
  exit 1
fi
echo "✓ checksum verified"

tar -xzf "$tmpdir/$asset" -C "$tmpdir"

mkdir -p "$INSTALL_DIR"
for bin in "$BIN_NAME" "$SERVER_BIN_NAME"; do
  src="$tmpdir/$bin"
  if [[ ! -f "$src" ]]; then
    echo "✗ missing binary in archive: $bin" >&2
    exit 1
  fi
  mv "$src" "$INSTALL_DIR/$bin"
  chmod +x "$INSTALL_DIR/$bin"
  echo "✓ installed $bin to $INSTALL_DIR/$bin"
done

# PATH hint.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "⚠ $INSTALL_DIR is not on your PATH. Add it:";
     echo "    export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

echo "→ try: $BIN_NAME --help"
echo "→ try: $BIN_NAME try customer-service"
