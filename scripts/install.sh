#!/usr/bin/env bash
# scripts/install.sh — installer scaffold for HnsX CLI.
#
# v0.8 ships this script as the canonical `curl -sSL hnsx.dev/install.sh | sh`
# entrypoint. It is intentionally conservative:
#   - resolves the latest release tag via the GitHub API
#   - picks the matching asset for the host OS/arch
#   - downloads to a temp file, verifies it is executable, and renames it
#     into $HOME/.local/bin/hnsx (creating the dir if needed)
#   - prints next-step guidance (add to PATH if needed)
#
# Production hardening (checksums, cosign verification, system paths) lives
# in the release pipeline; this file is the canonical in-repo copy that the
# website links to.

set -euo pipefail

REPO="hnsx-io/hnsx"
BIN_NAME="hnsx"
INSTALL_DIR="${HNSX_INSTALL_DIR:-$HOME/.local/bin}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "✗ missing: $1" >&2; exit 1; }; }

need curl
need tar

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
echo "→ downloading $url"

tmpdir="$(mktemp -d -t hnsx-install.XXXXXX)"
trap 'rm -rf "$tmpdir"' EXIT
curl -fsSL "$url" -o "$tmpdir/$asset"
tar -xzf "$tmpdir/$asset" -C "$tmpdir"

mkdir -p "$INSTALL_DIR"
mv "$tmpdir/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo "✓ installed $BIN_NAME to $INSTALL_DIR/$BIN_NAME"

# PATH hint.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "⚠ $INSTALL_DIR is not on your PATH. Add it:";
     echo "    export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac

echo "→ try: $BIN_NAME --help"