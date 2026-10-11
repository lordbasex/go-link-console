# Building, testing and releasing

## Requirements

| Part | Needs |
|---|---|
| Device | Go (version in `backend-device/go.mod`), a C compiler, `pkg-config`, **libvpx** and **libopus** (cgo) |
| Device window on Linux | OpenGL, X11 and Wayland development packages (`libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev wayland-protocols`) |
| Website | Node.js (version in `frontend/.nvmrc`) and npm |
| Cross builds | Docker with `buildx` (Linux and Windows device builds run in containers); `nasm` for the macOS Intel build (`brew install nasm`) |
| Willy Maker's board model (only to rebuild it) | Emscripten from [emsdk](https://emscripten.org/docs/getting_started/downloads.html) and a host C compiler (see below) |
| Device builds (`make device…`, `make release`) | A clone of [golink-hd](https://github.com/lordbasex/golink-hd), go-link HD's engine, next to this repository (`../golink-hd`) or `GOLINK_HD_DIR`: every build ships its library, compiled from that clone's committed HEAD |
| End-to-end tests | A clone of [signalhub](https://github.com/lordbasex/signalhub) next to this repository (`../signaling`) or `SIGNALING_DIR` |

Install the device's C libraries:

| System | Command |
|---|---|
| macOS | `brew install libvpx opus pkg-config` |
| Debian/Ubuntu | `sudo apt install libvpx-dev libopus-dev pkg-config` |
| Windows (MSYS2) | `pacman -S mingw-w64-x86_64-libvpx mingw-w64-x86_64-opus mingw-w64-x86_64-pkg-config` |

These libraries are needed **only to build** and to run `go test`. The release builds link them statically (on macOS, built from source by `build/macos/static-libs.sh`), so the binaries run without them.

### The power-on test's WebAssembly

Willy Maker's power-on test runs a 68000 and a CPS-1 board model compiled to WebAssembly (`frontend/packages/cps1-sim`, see [the architecture](willy-maker/architecture.md#the-power-on-test-go-linkcps1-sim-and-power)). The built file, `wasm/cps1sim.wasm`, is **committed**: the website build, the tests and CI use it as it is and never need Emscripten. Only a change to `c/` or `musashi/` needs a rebuild:

```bash
git clone https://github.com/emscripten-core/emsdk.git ~/emsdk
~/emsdk/emsdk install latest && ~/emsdk/emsdk activate latest
source ~/emsdk/emsdk_env.sh                 # puts emcc on PATH (and sets EMSDK)
cd frontend
npm run wasm -w @go-link/cps1-sim           # rebuilds wasm/cps1sim.wasm and cps1sim.json
npm run wasm:check -w @go-link/cps1-sim     # rebuilds in a temporary folder and compares
```

`tools/build.mjs` uses `emcc` from `PATH`, or `$EMSDK/upstream/emscripten/emcc` when only `EMSDK` is set. `wasm/cps1sim.json` records the `emcc` version that built the committed file; a different version may give different bytes, so `wasm:check` is meant for the machine that last rebuilt it, not for CI.

## Makefile

`make help` lists everything.

| Command | What it does |
|---|---|
| `make all` | Website and device for every platform |
| `make web-build` | Builds the website for `wss://signal.go-link.org/ws` (`SIGNAL_URL=…` changes it) into `frontend/apps/web/dist`, without source maps |
| `make web-deploy` | Builds and uploads the website with the local hosting file (see [deploy.md](deploy.md#website)) |
| `make device` | The device for macOS (universal), Linux (window and headless) and Windows, amd64 and arm64, into `dist/device/` |
| `make device-darwin-universal` | The macOS app for Intel and Apple silicon in one binary, for macOS 12 or later, from any Mac |
| `make device-darwin-amd64` | One platform. Also `-darwin-arm64`, `-linux-amd64`, `-linux-arm64`, `-linux-amd64-headless`, `-linux-arm64-headless`, `-windows-amd64`, `-windows-arm64` |
| `make device-dmg` | The universal macOS app in a drag-to-Applications `.dmg` |
| `make panel` | Builds the website into `backend-device/web/panel/dist`, the panel that headless devices serve (device builds do it when it is missing) |
| `make device-docker` | The `go-link-console` image (headless, panel on :7373) in the local Docker |
| `make device-docker-oci` | The same image for amd64 and arm64, as an OCI archive in `dist/docker/` |
| `make e2e` | End-to-end tests in a real browser |
| `VERSION=x.y.z make release` | A full release (below) |

How the device is built on each system (cgo: libvpx, libopus and, with the window, OpenGL):

- **macOS:** on any Mac, both architectures: clang builds x86_64 and arm64 with the same SDK. `backend-device/build/macos/static-libs.sh` builds libvpx and Opus from source (pinned versions and SHA-256) for each architecture and for macOS 12, cached in `dist/.macos-libs/`, and `lipo` joins both device binaries into one universal app. Homebrew's copies are not used: they only exist for the Mac's own CPU and require the macOS they were built on.
- **Linux:** in Docker (`backend-device/build/linux.Dockerfile`), the other architecture through QEMU, with static libvpx and Opus.
- **Windows:** in Docker with llvm-mingw (`backend-device/build/windows.Dockerfile`), with libvpx and libopus built statically: the `.exe` needs no DLLs.

What each binary needs from the system:

| Binary | System libraries |
|---|---|
| Windows | Nothing extra (Windows 10/11 DLLs only) |
| macOS | Nothing extra (macOS frameworks) |
| Linux headless (Raspberry Pi, Docker) | `libc` and `libm` |
| Linux with the window | Also `libGL` and `libwayland-client`, which any desktop has |

The emulator core is not in any binary: it is downloaded on first use (see [emulator.md](emulator.md)).

## macOS app

On macOS the device ships as one universal **`go-link-console.app`** (Intel and Apple silicon, macOS 12 or later), not a bare executable (which the Finder would open through Terminal). It has its `Info.plist` (`org.go-link.console`), an `.icns` icon made from the logo and an ad hoc signature.

`make device-dmg` builds `dist/device/go-link-<version>-macos-universal.dmg` (with its `.sha256`): the app on the left, a shortcut to Applications on the right, on a dark background with an amber arrow (`backend-device/build/macos/dmg-background.swift`). `backend-device/build/macos/make-dmg.sh` uses only macOS tools (`hdiutil`, `SetFile`, `osascript`, `swift`). The first time, macOS asks for permission for the terminal to control the Finder (the window layout); without it the `.dmg` is made without the background.

## Releases

`VERSION=0.1.0 make release` (or `./scripts/release.sh`) builds everything that goes in a GitHub release, into `dist/release/v0.1.0/`:

| File | For |
|---|---|
| `go-link-console-v0.1.0-macos-universal.dmg` | macOS 12 or later, Intel and Apple silicon: drag to Applications |
| `go-link-console-v0.1.0-windows-amd64.zip` / `-arm64.zip` | Windows: the `.exe`, no console |
| `go-link-console-v0.1.0-linux-amd64.tar.gz` / `-arm64.tar.gz` | Linux desktops |
| `go-link-console-v0.1.0-linux-amd64-headless.tar.gz` / `-arm64-headless.tar.gz` | Raspberry Pi and servers (no window, web panel) |
| `go-link-console-v0.1.0-docker.oci.tar.gz` (skipped with `DOCKER=0`) | The Docker image (amd64 and arm64), for `docker load` without internet; it loads as `go-link-console:v0.1.0`. Publishing the release also runs `.github/workflows/docker.yml`, which pushes the same image to `ghcr.io/lordbasex/go-link-console` (`:v0.1.0` and `:latest`) |
| `go-link-console-v0.1.0-android.apk` (skipped with `ANDROID=0`) | **go-link Console Player** for Android, signed with the release key (`ANDROID_SIGNING`, see [mobile.md](mobile.md)) |
| `SHA256SUMS` | To verify the downloads |

The release also writes `frontend/apps/web/src/release.json` (version and download sizes) for the website's downloads; commit it with `Casks/go-link-console.rb` and deploy the website.

It also updates `Casks/go-link-console.rb` (Homebrew: `brew install --cask go-link` from the repository's tap) and, with `gh`, creates the `v0.1.0` release on `lordbasex/go-link-console` (`REPO=…` changes it) with every file and generated notes. `SKIP_BUILD=1` packs what is already in `dist/device`, and `GITHUB_RELEASE=0` only builds the files.

**Development mode** (no `CODESIGN_IDENTITY`, the default): the macOS app has an ad hoc signature and the GitHub release is a **pre-release**. On another Mac, macOS blocks it the first time: open it, then System Settings › Privacy & Security › Open Anyway (right click › Open on macOS 14 and earlier). Releases before 0.3.0 were made this way.

**With an Apple Developer ID** (every release since 0.3.0):

1. Install the "Developer ID Application: Name (TEAMID)" certificate in the keychain.
2. Store the notarization credentials once: `xcrun notarytool store-credentials "go-link-notary" --apple-id you@example.com --team-id TEAMID --password <app-specific-password>`.
3. `VERSION=0.1.0 CODESIGN_IDENTITY="Developer ID Application: Name (TEAMID)" NOTARY_PROFILE=go-link-notary make release`

The app is then signed with the hardened runtime and `backend-device/build/macos/entitlements.plist` (`disable-library-validation`, because the device loads the libretro core, which is downloaded separately and not signed by us). Notarization is done twice (`build/macos/notarize.sh`): first the app, whose ticket is stapled inside `go-link-console.app` (the copy in /Applications then opens without asking Apple's servers, offline too), then the `.dmg` made with that app, signed, notarized and stapled. The release is no longer a pre-release.

## Tests

| Where | Command |
|---|---|
| Device | `cd backend-device && go test -race ./...` |
| One device test | `go test -race -run TestName ./internal/services/` |
| Headless build check | `go build -tags headless ./cmd/device` |
| Website | `cd frontend && npm test && npm run typecheck` |
| One website test | `npx vitest run apps/web/src/app.test.tsx -t "<name>"` |

Before pushing Go code, in `backend-device/`: `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...` and `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`. CI fails on any of them.

### End-to-end tests

`e2e/` drives a **real Chromium** (Playwright) against its own stack, started by `e2e/global-setup.ts`: a signalhub on `:8191` (built from `../signaling` or `SIGNALING_DIR`), a headless device on `:7391` with a temporary `HOME` (it never touches anyone's configuration, rooms or history) and the website on `:5191`. It first builds the website and embeds it in the device as its panel, so the latest code is tested. It covers:

1. The landing page: menu and the "Join a game" dialog.
2. Linking with the 9-digit code and the live dashboard (data over WebRTC).
3. Reload: the remembered browser comes back, the device proves itself, and only then does the browser show its token.
4. The test pattern room: the video plays.
5. Invitations: a guest joins with code and PIN and sees the video; another with the same PIN sees "someone already joined with this invitation".
6. The device's local panel with its token (challenge and proof), and a room played from the panel.
7. Accessibility: axe on every page.

```bash
make e2e                                          # or, inside e2e/:
npm test                                          # the web project
npm run test:android                              # the Android app on an emulator (docs/mobile.md)
npm run test:android:camera                       # its QR scanner on the emulator's virtual camera (restarts the emulator)
npm run shots                                     # the landing page's screenshots (below)
npx playwright test --project=web tests/<file> -g "<name>"   # a single test
npm run report                                    # the HTML report
```

Rooms with real games are not covered, because they need the core and ROMs.

### Landing page screenshots

The landing page shows real screenshots (`frontend/apps/web/public/shots/<lang>/<name>.webp`, sizes in `frontend/apps/web/src/components/landing/shots.json`). `cd e2e && npm run shots` makes them all again, in English, Spanish and Portuguese, with the `shots` Playwright project (never part of `npm test` or CI):

- **Device window:** the Go test `TestShots` renders the real window with Fyne's test driver and made-up status data (`GOLINK_SHOTS=<dir> go test ./internal/gui -run TestShots` in `backend-device/`; skipped without the variable): the pairing view, Overview and MAME › ROMs.
- **Website:** the e2e stack with `E2E_SHOTS=1`: the device's throwaway `HOME` gets an **invented** ROM library first (`go run ./cmd/shotseed`, `backend-device/internal/shots`: made-up games, set names, makers and geometric covers, never a real game), and the device is built with the released version. The pictures: the code form, the test pattern room, the invitation, My device and its ROMs tab.
- **Player app:** when an emulator or phone is in `adb devices` and the debug APK is built, the app in each language (`cmd locale set-app-locales`, Android 13 or newer) with a clean status bar (demo mode): home, PIN, the room and its chat. Another build of the app installed there (a release) is kept and put back at the end.

Before every web picture the page's computer name, user name and local paths are replaced (the test stack's addresses read as the public ones), and a picture that still shows any of them or an IPv4 address fails. Only the test pattern room is ever played. Pictures are converted with `cwebp` (desktop 1440 px wide, phones 600 px, each under 150 KB). Look at every picture before publishing them.

### Trailer and demo video

`video/` makes go-link's trailer (about 50 s) and its demo (about 3 minutes, seven chapters) with Remotion, from takes of the real app:

1. **Takes:** `cd e2e && npm run video` (`VIDEO_LANG=es|pt` for the other languages) records the website with the `video` Playwright project, with the invented ROM library of the screenshots, into `video/public/takes/<lang>/` (landing, linking, the test pattern room, a guest, the phone console, Willy Maker and the network report).
2. **Narrator:** `scripts/voice.py` (Kokoro, `KOKORO_DIR`) turns `src/narration.json` (trailer) or `src/demo-narration.json` (demo, `VOICE_SET=demo`) into one WAV per line at -16 LUFS, and writes how long each line lasts; the scenes stretch to fit their line.
3. **Render:** `npm run trailers` (every language and subtitle version) or `npm run demos` (the demo in English, Spanish and Portuguese; `npm run demo`, `demo:es`, `demo:pt` one at a time) write `dist/video/*.mp4`, plus SubRip subtitles (`npm run srt`) from the same timelines (`src/timeline.ts`, `src/demoTimeline.ts`). Where each shot starts in its take is set in those timelines (the test pattern's clock in the room take tells the time); every language uses its own takes (`VIDEO_LANG`), and a shot can start elsewhere in one language's take (`fromBy`). The chapter titles and the narration are translated in `demoTimeline.ts` and `demo-narration.json` (Spanish with "tú", Brazilian Portuguese), and the narrator reads web addresses with the language's word for the dots.

Takes, voices and music stay out of the repository; the music is the author's own.

## Continuous integration

`.github/workflows/ci.yml` runs on every push to `main` and every pull request, with read-only permissions:

| Job | What it does |
|---|---|
| `device` | Installs libvpx, Opus and the window libraries, then `gofmt`, `go vet`, `go test -race`, a headless build and `govulncheck` |
| `web` | `npm ci`, typecheck, tests, build and `npm audit` |
| `e2e` | Clones signalhub and runs the end-to-end tests, after the other jobs pass |
| `secrets` | `gitleaks` over the whole history |

`.github/workflows/android-e2e.yml` runs the Android app's end-to-end test on an emulator (KVM), weekly and by hand, not on every push.
