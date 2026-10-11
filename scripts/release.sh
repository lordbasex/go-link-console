#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Cuts a release of the go-link device: builds every platform, packs each
# one, writes SHA256SUMS, updates the Homebrew cask and, when `gh` is
# available, creates the GitHub release with every file.
#
# Development mode (the default, while there is no Developer ID): the macOS
# app has an ad hoc signature and the GitHub release is marked as a
# pre-release. With a Developer ID the app is signed with the hardened
# runtime and notarized with its ticket stapled (it opens offline once
# copied to /Applications), then the .dmg made with it is notarized too:
#
#   VERSION=0.1.0 ./scripts/release.sh
#   VERSION=0.1.0 CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)" \
#     NOTARY_PROFILE=go-link-notary ./scripts/release.sh
#
# Other knobs: REPO=owner/name (default lordbasex/go-link-console), SKIP_BUILD=1
# (pack what is already in dist/device), DOCKER=0 (skip the Docker image, built by default as
# an OCI archive), ANDROID=0 (skip the signed Android APK, built by default:
# it needs ANDROID_SIGNING, see mobile/android/README.md), GITHUB_RELEASE=0
# (only the files).
#
# Outputs: dist/release/v<version>/
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${VERSION:?Set VERSION, e.g. VERSION=0.1.0}"
VERSION="${VERSION#v}"
TAG="v$VERSION"
REPO="${REPO:-lordbasex/go-link-console}"
CODESIGN_IDENTITY="${CODESIGN_IDENTITY:--}"
DEVICE="$ROOT/dist/device"
OUT="$ROOT/dist/release/$TAG"
NAME="go-link-console-$TAG"

if [[ "$CODESIGN_IDENTITY" == "-" || -z "${NOTARY_PROFILE:-}" ]]; then
  DEV=1
  echo "▶ Development release: ad hoc signature, not notarized (a pre-release on GitHub)"
else
  DEV=0
  echo "▶ Signed release: $CODESIGN_IDENTITY, notarized with $NOTARY_PROFILE"
fi

if [[ "${SKIP_BUILD:-0}" != "1" ]]; then
  echo "▶ Building every platform ($TAG)"
  "$ROOT/scripts/third-party-notices.sh"
  make -C "$ROOT" panel
  make -C "$ROOT" device VERSION="$TAG" CODESIGN_IDENTITY="$CODESIGN_IDENTITY"
  if [[ "$(uname -s)" == "Darwin" ]]; then
    app="$DEVICE/darwin-universal/go-link-console.app"
    if [[ "$DEV" == "0" ]]; then
      # the app's own ticket, stapled before it goes into the disk image
      "$ROOT/backend-device/build/macos/notarize.sh" "$app"
    fi
    # make-dmg.sh directly: `make device-dmg` would build the app again and lose its ticket
    CODESIGN_IDENTITY="$CODESIGN_IDENTITY" "$ROOT/backend-device/build/macos/make-dmg.sh" "$app" \
      "$DEVICE/$NAME-macos-universal.dmg"
  fi
  if [[ "${DOCKER:-1}" == "1" ]]; then
    make -C "$ROOT" device-docker-oci VERSION="$TAG"
  fi
  if [[ "${ANDROID:-1}" == "1" ]]; then
    make -C "$ROOT" android-apk VERSION="$TAG"
  fi
fi

rm -rf "$OUT"
mkdir -p "$OUT"

# macOS: one universal .dmg (Intel + Apple silicon, macOS 12 or later).
dmg="$DEVICE/$NAME-macos-universal.dmg"
if [[ -f "$dmg" ]]; then
  if [[ "$DEV" == "0" ]]; then
    "$ROOT/backend-device/build/macos/notarize.sh" "$dmg"
  fi
  cp "$dmg" "$OUT/"
else
  echo "  (no macOS build: make it on a Mac)"
fi

# Linux: a .tar.gz per build; "headless" is the one without a window
# (Raspberry Pi, servers), which opens the web panel.
for variant in linux-amd64 linux-arm64 linux-amd64-headless linux-arm64-headless; do
  bin="$DEVICE/$variant/go-link-console"
  [[ -f "$bin" ]] || { echo "  (no $variant build, skipped)"; continue; }
  cp "$ROOT/LICENSE" "$ROOT/THIRD_PARTY_NOTICES.md" "$DEVICE/$variant/"
  [[ -f "$DEVICE/$variant/libgolinkhd.so" ]] || { echo "✗ $variant has no go-link HD engine (libgolinkhd.so)"; exit 1; }
  tar -C "$DEVICE/$variant" -czf "$OUT/$NAME-$variant.tar.gz" go-link-console libgolinkhd.so LICENSE THIRD_PARTY_NOTICES.md
done

# Windows: a .zip with the .exe (no console; the log goes to a file).
for arch in amd64 arm64; do
  exe="$DEVICE/windows-$arch/go-link-console.exe"
  [[ -f "$exe" ]] || { echo "  (no windows-$arch build, skipped)"; continue; }
  cp "$ROOT/LICENSE" "$ROOT/THIRD_PARTY_NOTICES.md" "$DEVICE/windows-$arch/"
  [[ -f "$DEVICE/windows-$arch/golinkhd.dll" ]] || { echo "✗ windows-$arch has no go-link HD engine (golinkhd.dll)"; exit 1; }
  (cd "$DEVICE/windows-$arch" && zip -q -j "$OUT/$NAME-windows-$arch.zip" go-link-console.exe golinkhd.dll LICENSE THIRD_PARTY_NOTICES.md)
done

oci="$ROOT/dist/docker/go-link-console-$TAG.oci.tar"
if [[ -f "$oci" ]]; then
  gzip -c "$oci" > "$OUT/$NAME-docker.oci.tar.gz"
fi

# Android: the go-link Console Player app, signed with the release key.
apk="$ROOT/dist/android/go-link-player-$TAG.apk"
if [[ -f "$apk" ]]; then
  cp "$apk" "$OUT/$NAME-android.apk"
else
  echo "  (no Android APK: ANDROID=0 or not built)"
fi

# The licenses also go as their own files (the .dmg has them inside the app).
cp "$ROOT/LICENSE" "$OUT/LICENSE.txt"
cp "$ROOT/THIRD_PARTY_NOTICES.md" "$OUT/THIRD_PARTY_NOTICES.md"

echo "▶ SHA256SUMS"
(cd "$OUT" && shasum -a 256 -- * > SHA256SUMS && cat SHA256SUMS)

echo "▶ Website downloads (frontend/apps/web/src/release.json)"
"$ROOT/scripts/release-info.sh" "$OUT"

echo "▶ Updating Casks/go-link-console.rb"
CASK="$ROOT/Casks/go-link-console.rb"
sed -i '' -e "s/^  version \".*\"/  version \"$VERSION\"/" "$CASK"
f="$OUT/$NAME-macos-universal.dmg"
if [[ -f "$f" ]]; then
  sum="$(shasum -a 256 "$f" | awk '{print $1}')"
  sed -i '' -E "s/^  sha256 \"[0-9a-f]{64}\"/  sha256 \"$sum\"/" "$CASK"
fi

# gh may live in a PATH only the login shell knows (Homebrew, ~/.local/bin).
GH="$(command -v gh || /bin/zsh -lc 'command -v gh' 2>/dev/null || true)"
if [[ -n "$GH" && "${GITHUB_RELEASE:-1}" == "1" ]]; then
  flags=(--repo "$REPO" --title "go-link Console $VERSION" --generate-notes)
  [[ "$DEV" == "1" ]] && flags+=(--prerelease)
  echo "▶ Creating the GitHub release $TAG on $REPO"
  "$GH" release create "$TAG" "$OUT"/* "${flags[@]}" \
    || echo "  (gh release failed or already exists: upload dist/release/$TAG by hand)"
else
  echo "▶ GitHub release skipped ($([[ -n "$GH" ]] && echo 'GITHUB_RELEASE=0' || echo 'gh not installed')): upload dist/release/$TAG to https://github.com/$REPO/releases/new (tag $TAG)"
fi
echo "✔ Release files in dist/release/$TAG"
exit 0
