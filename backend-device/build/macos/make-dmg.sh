#!/usr/bin/env bash
# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# Packs go-link-console.app into a drag-to-install disk image. The mounted volume
# shows the go-link icon, a background with an arrow, go-link-console.app on the
# left and an Applications shortcut on the right. It needs only macOS tools
# (hdiutil, SetFile, osascript, swift). The Finder layout step asks once for
# permission to automate the Finder; if it is denied the image is still
# made, just without the window layout.
#
# Usage: build/macos/make-dmg.sh APP OUT.dmg
#        CODESIGN_IDENTITY="Developer ID Application: ..." build/macos/make-dmg.sh APP OUT.dmg
set -euo pipefail

APP="${1:?usage: make-dmg.sh APP OUT.dmg}"
DMG="${2:?usage: make-dmg.sh APP OUT.dmg}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="$(dirname "$DMG")/.dmg-work"
VOLNAME="go-link Console"
RW="$WORK/go-link-rw.dmg"
STAGE="$WORK/root"
ICON="$APP/Contents/Resources/go-link.icns"

[[ -d "$APP" ]] || { echo "✘ $APP not found: build the app first" >&2; exit 1; }

# A mounted volume with the same name would confuse the Finder step.
if [[ -d "/Volumes/$VOLNAME" ]]; then hdiutil detach "/Volumes/$VOLNAME" -quiet || true; fi

echo "▶ Background"
rm -rf "$WORK" "$DMG"
mkdir -p "$STAGE/.background"
swift "$HERE/dmg-background.swift" "$WORK/background.tiff" >/dev/null

echo "▶ Staging"
ditto "$APP" "$STAGE/go-link-console.app"
ln -s /Applications "$STAGE/Applications"
cp "$WORK/background.tiff" "$STAGE/.background/background.tiff"

echo "▶ Creating a writable image"
hdiutil create -srcfolder "$STAGE" -volname "$VOLNAME" -fs HFS+ -fsargs "-c c=64,a=16,e=16" \
  -format UDRW -size 200m "$RW" -quiet
MOUNT_OUT="$(hdiutil attach -readwrite -noverify -noautoopen "$RW")"
DEV="$(echo "$MOUNT_OUT" | grep -E '^/dev/' | head -1 | awk '{print $1}')"
VOL="/Volumes/$VOLNAME"

echo "▶ Finder layout"
if ! osascript <<APPLESCRIPT
tell application "Finder"
  tell disk "$VOLNAME"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set the bounds of container window to {200, 120, 860, 520}
    set opts to the icon view options of container window
    set arrangement of opts to not arranged
    set icon size of opts to 128
    set text size of opts to 13
    set background picture of opts to file ".background:background.tiff"
    set position of item "go-link-console.app" of container window to {165, 190}
    set position of item "Applications" of container window to {495, 190}
    close
    open
    update without registering applications
    delay 1
    close
  end tell
end tell
APPLESCRIPT
then
  echo "  (Finder layout skipped: allow the terminal to control Finder in System Settings › Privacy & Security › Automation and run it again)"
fi

# The volume icon goes LAST, after every Finder call: the Finder's update
# of the disk deletes .VolumeIcon.icns and clears the flag, and
# hdiutil -srcfolder does not copy it either. The C flag makes the Finder
# use it.
cp "$ICON" "$VOL/.VolumeIcon.icns"
SetFile -a C "$VOL"
SetFile -a V "$VOL/.background" 2>/dev/null || true
sync
sleep 1
hdiutil detach "$DEV" -quiet

echo "▶ Compressing"
hdiutil convert "$RW" -format UDZO -imagekey zlib-level=9 -o "$DMG" -quiet
rm -rf "$WORK"

if [[ -n "${CODESIGN_IDENTITY:-}" && "$CODESIGN_IDENTITY" != "-" ]]; then
  echo "▶ Signing the image"
  codesign --sign "$CODESIGN_IDENTITY" --timestamp "$DMG"
fi

shasum -a 256 "$DMG" | tee "$DMG.sha256"
echo "✔ $DMG"
