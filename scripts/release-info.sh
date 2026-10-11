#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Writes frontend/apps/web/src/release.json from a release folder: the
# version and the size of each download, so the website links the latest
# release (the downloads card and the how-it-works wizard).
#
#   scripts/release-info.sh dist/release/v0.1.2
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIR="${1:?release folder, e.g. dist/release/v0.1.2}"
TAG="$(basename "$DIR")"
OUT="$ROOT/frontend/apps/web/src/release.json"
{
  printf '{\n  "version": "%s",\n  "assets": {\n' "$TAG"
  first=1
  for f in "$DIR"/go-link-console-"$TAG"-*; do
    [[ -f "$f" ]] || continue
    key="${f##*/go-link-console-$TAG-}"
    size=$(wc -c < "$f" | tr -d ' ')
    [[ $first == 1 ]] || printf ',\n'
    printf '    "%s": %s' "$key" "$size"
    first=0
  done
  printf '\n  }\n}\n'
} > "$OUT"
echo "✓ $OUT ($TAG)"
