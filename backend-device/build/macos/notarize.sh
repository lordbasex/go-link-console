#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Sends the signed app or disk image to Apple's notary service, staples the
# ticket to it and checks that Gatekeeper accepts it. The app must be signed
# with a Developer ID and the hardened runtime (make ... CODESIGN_IDENTITY=...).
#
# The release notarizes twice: the app first (its ticket stapled into
# go-link-console.app, so the copy in /Applications opens without asking Apple's
# servers, offline too), then the disk image made with that app.
#
# One time, before the first release:
#   1. Apple Developer Program membership.
#   2. The "Developer ID Application: Your Name (TEAMID)" certificate in the
#      login keychain.
#   3. An app-specific password stored for notarytool:
#        xcrun notarytool store-credentials "go-link-notary" \
#          --apple-id you@example.com --team-id TEAMID --password <app-specific-password>
#
# Usage: NOTARY_PROFILE=go-link-notary build/macos/notarize.sh go-link-console.app
#        NOTARY_PROFILE=go-link-notary build/macos/notarize.sh go-link.dmg
set -euo pipefail

TARGET="${1:?usage: notarize.sh APP|DMG}"
TARGET="${TARGET%/}"
: "${NOTARY_PROFILE:?Set NOTARY_PROFILE to the notarytool keychain profile name}"

case "$TARGET" in
*.app)
  # notarytool takes a zip of the app (ditto keeps its signature and links)
  ZIP="$(mktemp -d)/$(basename "$TARGET" .app).zip"
  ditto -c -k --keepParent "$TARGET" "$ZIP"
  echo "▶ Submitting $(basename "$TARGET") to Apple's notary service (a few minutes)"
  xcrun notarytool submit "$ZIP" --keychain-profile "$NOTARY_PROFILE" --wait
  rm -f "$ZIP"
  echo "▶ Stapling the ticket to the app"
  xcrun stapler staple "$TARGET"
  xcrun stapler validate "$TARGET"
  spctl --assess --type execute --verbose=2 "$TARGET"
  ;;
*.dmg)
  echo "▶ Submitting $(basename "$TARGET") to Apple's notary service (a few minutes)"
  xcrun notarytool submit "$TARGET" --keychain-profile "$NOTARY_PROFILE" --wait
  echo "▶ Stapling the ticket to the disk image"
  xcrun stapler staple "$TARGET"
  xcrun stapler validate "$TARGET"
  spctl --assess --type open --context context:primary-signature --verbose=2 "$TARGET"
  shasum -a 256 "$TARGET" > "$TARGET.sha256"
  ;;
*)
  echo "notarize.sh: give a .app or a .dmg" >&2
  exit 1
  ;;
esac
echo "✔ Notarized: $TARGET"
