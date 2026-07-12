#!/usr/bin/env bash
# sync-docs.sh — Keep website/docs/ as a mirror of documents/.
#
# documents/ is the single source of truth for user-facing docs.
# website/docs/ is the build artifact consumed by Rspress to produce
# the GitHub Pages site. The website package needs the markdown files
# alongside its own index.md / blog/ to satisfy Rspress's file-based
# routing, so we sync them in.
#
# Usage:
#   scripts/sync-docs.sh             # mirror documents/ -> website/docs/
#   scripts/sync-docs.sh --check     # exit non-zero if website/docs/ is out of sync
#   scripts/sync-docs.sh --clean     # remove generated subdirs before syncing
#
# See docs/PROVENANCE.md and documents/README.md for the model.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/documents"
DST="$ROOT/website/docs"

CHECK_ONLY=false
CLEAN=false
for arg in "$@"; do
  case "$arg" in
    --check) CHECK_ONLY=true ;;
    --clean) CLEAN=true ;;
    -h|--help)
      sed -n '2,/^$/s/^# \{0,1\}//p' "$0"
      exit 0
      ;;
    *)
      echo "Unknown flag: $arg" >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "$SRC" ]]; then
  echo "error: source $SRC does not exist" >&2
  exit 1
fi
if [[ ! -d "$DST" ]]; then
  echo "error: destination $DST does not exist" >&2
  exit 1
fi

# Subdirectories of documents/ that get mirrored into website/docs/.
# Each entry becomes its own subdirectory under website/docs/. Add a
# new line when you add a new top-level public doc folder.
SYNC_DIRS=(guide blog know-how)

mirror_one() {
  local name="$1"
  local src_dir="$SRC/$name"
  local dst_dir="$DST/$name"

  if [[ "$CLEAN" == true && -d "$dst_dir" ]]; then
    rm -rf "$dst_dir"
  fi
  mkdir -p "$dst_dir"

  if [[ "$CHECK_ONLY" == true ]]; then
    # rsync --dry-run returns 0 even with diffs; parse output for changed files.
    if rsync -a --dry-run --itemize-changes "$src_dir/" "$dst_dir/" 2>/dev/null \
         | grep -qE '^>f'; then
      echo "website/docs/$name is out of sync with documents/$name" >&2
      return 1
    fi
    return 0
  fi

  rsync -a --delete --exclude='.DS_Store' "$src_dir/" "$dst_dir/"
  echo "synced documents/$name -> website/docs/$name"
}

# Top-level files in documents/ that get mirrored as-is into website/docs/
# (preserving the filename). Add when you add a new top-level public doc.
SYNC_FILES=(vision.md architecture.md api-reference.md console-design.md provenance.md README.md)

mirror_file() {
  local name="$1"
  local src_file="$SRC/$name"
  local dst_file="$DST/$name"

  if [[ ! -f "$src_file" ]]; then
    # Top-level files are optional — silently skip if not present.
    return 0
  fi

  if [[ "$CHECK_ONLY" == true ]]; then
    if ! cmp -s "$src_file" "$dst_file" 2>/dev/null; then
      echo "website/docs/$name is out of sync with documents/$name" >&2
      return 1
    fi
    return 0
  fi

  cp "$src_file" "$dst_file"
  echo "synced documents/$name -> website/docs/$name"
}

status=0
for d in "${SYNC_DIRS[@]}"; do
  mirror_one "$d" || status=1
done
for f in "${SYNC_FILES[@]}"; do
  mirror_file "$f" || status=1
done

if [[ "$CHECK_ONLY" == true ]]; then
  if [[ $status -eq 0 ]]; then
    echo "website/docs/ is in sync with documents/"
  else
    echo "" >&2
    echo "run: scripts/sync-docs.sh" >&2
  fi
  exit $status
fi

exit 0