# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

# Homebrew cask for the go-link device on macOS. Install from this repo's tap:
#   brew tap lordbasex/go-link-console https://github.com/lordbasex/go-link-console
#   brew trust --cask lordbasex/go-link-console/go-link-console   (newer Homebrew asks to trust third-party taps once)
#   brew install --cask go-link-console
# One universal app (Intel + Apple silicon). scripts/release.sh rewrites
# version and sha256 on every release.
cask "go-link-console" do
  version "0.4.4"
  sha256 "b26a55ad62c8580a9032d4a003785c59a7e0adbd38e98da4ce407f6ccb5ebe49"

  url "https://github.com/lordbasex/go-link-console/releases/download/v#{version}/go-link-console-v#{version}-macos-universal.dmg"
  name "go-link Console"
  desc "Retro arcade that streams live from your computer to your friends' browsers"
  homepage "https://console.go-link.org"

  depends_on macos: :monterey

  app "go-link-console.app"
  binary "#{appdir}/go-link-console.app/Contents/MacOS/go-link-console", target: "go-link-console"

  # Only the device's settings and log; ~/go-link-console also holds the host's own
  # ROMs and saved games, which are never removed.
  zap trash: [
    "~/Library/Application Support/go-link-console",
    "~/go-link-console/logs",
  ]
end
