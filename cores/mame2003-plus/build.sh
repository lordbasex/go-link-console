#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Builds the mame2003-plus libretro core with go-link's patches, without
# touching the upstream source: it clones the commit named in UPSTREAM,
# applies patches/*.patch in order and runs the core's own Makefile.
#
#   ./build.sh [platform]      # platform: osx, unix, win (default: this machine)
#
# The core lands in ./out/. A patch that no longer applies stops the build
# with its name: refresh it against the new UPSTREAM commit.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="https://github.com/libretro/mame2003-plus-libretro.git"
commit="$(tr -d '[:space:]' < "$here/UPSTREAM")"
work="$here/work"
out="$here/out"

platform="${1:-}"
if [ -z "$platform" ]; then
	case "$(uname -s)" in
	Darwin) platform=osx ;;
	Linux) platform=unix ;;
	MINGW* | MSYS* | CYGWIN*) platform=win ;;
	*) echo "unknown system: pass the platform" >&2; exit 1 ;;
	esac
fi

rm -rf "$work"
git init -q "$work"
git -C "$work" remote add origin "$repo"
git -C "$work" fetch -q --depth 1 origin "$commit"
git -C "$work" checkout -q FETCH_HEAD

# every patch is upstream since 2026-10-11, so the folder may be empty
shopt -s nullglob
for p in "$here"/patches/*.patch; do
	echo "applying $(basename "$p")"
	if ! git -C "$work" apply --whitespace=nowarn "$p"; then
		echo "the patch $(basename "$p") does not apply to $commit" >&2
		exit 1
	fi
done

jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
extra=()
if [ "$platform" = osx ]; then
	# The core's bundled zlib defines fdopen as NULL on macOS, which recent
	# SDKs reject: keep the system's fdopen (a build flag, not a source change).
	extra=(CC="${CC:-cc} -Dfdopen=fdopen")
fi
make -C "$work" -j"$jobs" platform="$platform" "${extra[@]}"

mkdir -p "$out"
cp "$work"/mame2003_plus_libretro.* "$out"/
echo "built: $(ls "$out")"
