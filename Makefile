# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
#
# go-link builds and deploys.
#
#   make all          web + device for every platform
#   make web-build    build both websites (frontend/apps/web/dist-site and dist-play)
#   make web-deploy   build and upload the website (deploy/local/hosting.mk)
#   make maker-build  build Willy Maker's site (frontend/willy-maker/dist)
#   make maker-deploy build and upload Willy Maker's site (deploy/local/hosting.mk)
#   make help         everything else

.PHONY: all e2e release legal golinkhd-src panel device-dmg device-darwin-universal FORCE device-docker device-docker-oci help info web-build web-deploy maker-build maker-deploy hosting-help \
	device device-windows device-windows-amd64 device-windows-arm64 clean android-debug android-apk

# The darwin and linux device targets are pattern rules (device-darwin-%,
# device-linux-%): make does not apply pattern rules to .PHONY targets, so
# they are not listed above.

# Project
PROJECT_NAME   = go-link
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# macOS signature: "-" (ad hoc) is a development build that only runs on
# this Mac without warnings. With CODESIGN_IDENTITY="Developer ID
# Application: Name (TEAMID)" the app is signed with the hardened runtime,
# ready to notarize (scripts/release.sh with NOTARY_PROFILE).
CODESIGN_IDENTITY ?= -
# The x.y.z macOS shows for the app: VERSION when it is one (a release,
# whose tag does not exist yet while it builds), else the latest vX.Y.Z
# tag, else 0.1.0.
SHORT_VERSION ?= $(shell echo '$(VERSION)' | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+$$' && echo '$(VERSION)' | sed 's/^v//' || git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null | sed 's/^v//' || true)
SHORT_VERSION := $(or $(SHORT_VERSION),0.1.0)
DIST           = $(CURDIR)/dist

# Website hosting is specific to each deployment and stays out of the
# repository: put your own `web-deploy` target in deploy/local/hosting.mk
# (gitignored). See docs/deploy.md.
-include deploy/local/hosting.mk

SIGNAL_URL    ?= wss://signal.go-link.org/ws
# Each site is built with the others' addresses, never a development one left in a .env.local
# (the website trusts MAKER_URL's origin for its /maker-bridge tab, the landing
# and the rooms' site trust each other's for the handoff).
MAKER_URL     ?= https://maker.go-link.org
SITE_URL      ?= https://console.go-link.org
PLAY_URL      ?= https://play.go-link.org

# Paths
WEB_DIR      = frontend
# The website is built twice: the landing, guide and tools (console.go-link.org) and
# the rooms and My device (play.go-link.org, also the device's local panel).
SITE_DIST    = $(WEB_DIR)/apps/web/dist-site
PLAY_DIST    = $(WEB_DIR)/apps/web/dist-play
MAKER_DIST   = $(WEB_DIR)/willy-maker/dist
DEVICE_DIR   = backend-device
DEVICE_OUT   = $(DIST)/device
DOCKER_OUT   = $(DIST)/docker
HOST_ARCH    = $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
HOST_OS      = $(shell uname -s | tr A-Z a-z)

# Output colors
GREEN  = \033[0;32m
YELLOW = \033[1;33m
RED    = \033[0;31m
NC     = \033[0m

.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Everything

all: web-build maker-build device
	@echo "$(GREEN)✓ go-link $(VERSION) built in $(DIST), $(SITE_DIST) and $(PLAY_DIST)$(NC)"

info:
	@echo "$(GREEN)$(PROJECT_NAME) $(VERSION)$(NC)"
	@echo "Signaling:   $(SIGNAL_URL)"
	@echo "Host:        $(HOST_OS)/$(HOST_ARCH)"

# ---------------------------------------------------------------------------
# Website

# The Node version in frontend/.nvmrc: nvm switches to it (installing it
# once if needed) just for these commands; without nvm, the Node in PATH.
NODE_ENV_SETUP = if [ -s "$${NVM_DIR:-$$HOME/.nvm}/nvm.sh" ]; then \
	. "$${NVM_DIR:-$$HOME/.nvm}/nvm.sh" && { nvm use --silent || nvm install; } >/dev/null; fi; \
	echo "Node $$(node -v)"

web-build:
	@echo "$(YELLOW)Building the website $(VERSION) for $(SIGNAL_URL)...$(NC)"
	rm -rf $(SITE_DIST) $(PLAY_DIST)
	@# The in-browser MP4 helper (Go compiled to WebAssembly) goes in public/mp4.
	$(MAKE) -C frontend/wasm/mp4 build
	@cd $(WEB_DIR) && $(NODE_ENV_SETUP) && npm ci --no-audit --no-fund && \
		VITE_SIGNAL_URL=$(SIGNAL_URL) VITE_MAKER_URL=$(MAKER_URL) VITE_SITE_URL=$(SITE_URL) VITE_PLAY_URL=$(PLAY_URL) \
		VITE_DEMO_DATA=false VITE_APP_VERSION=$(VERSION) \
		VITE_BUILD_DATE=$$(date -u +%Y-%m-%d) npm run build
	@find $(SITE_DIST) $(PLAY_DIST) -name "*.map" -delete
	@echo "$(GREEN)✓ Websites built in $(SITE_DIST) and $(PLAY_DIST)$(NC)"

# Willy Maker's own site (maker.go-link.org): a static site of its own, which
# reaches the device through the rooms' site /maker-bridge tab (PLAY_URL), and
# the games made before it moved through the landing's (SITE_URL).
maker-build:
	@echo "$(YELLOW)Building Willy Maker's site $(VERSION) for $(SITE_URL)...$(NC)"
	rm -rf $(MAKER_DIST)
	@cd $(WEB_DIR) && $(NODE_ENV_SETUP) && npm ci --no-audit --no-fund && \
		VITE_SITE_URL=$(SITE_URL) VITE_PLAY_URL=$(PLAY_URL) VITE_APP_VERSION=$(VERSION) npm run build -w @go-link/willy-maker
	@find $(MAKER_DIST) -name "*.map" -delete
	@echo "$(GREEN)✓ Willy Maker's site built in $(MAKER_DIST)$(NC)"

ifndef HOSTING_TARGETS
# Without deploy/local/hosting.mk there is nowhere to upload to: each site is
# a static folder ($(SITE_DIST), $(PLAY_DIST), $(MAKER_DIST)), for any static host.
web-deploy: web-build
	@echo "$(YELLOW)The websites are built in $(SITE_DIST) and $(PLAY_DIST). Upload them to your static host, or add a"
	@echo "web-deploy target to deploy/local/hosting.mk (see docs/deploy.md).$(NC)"

maker-deploy: maker-build
	@echo "$(YELLOW)Willy Maker's site is built in $(MAKER_DIST). Upload it to your static host, or add a"
	@echo "maker-deploy target to deploy/local/hosting.mk (see docs/deploy.md).$(NC)"

hosting-help:
	@echo "$(YELLOW)Hosting:$(NC) add deploy/local/hosting.mk (see docs/deploy.md)"
endif


# ---------------------------------------------------------------------------
# Device (cgo: libvpx, libopus and, with the window, OpenGL)

# End-to-end tests: a real Chromium against its own signaling server,
# headless device and website (e2e/). The first run downloads Chromium.
e2e:
	cd e2e && npm ci --no-audit --no-fund && npx playwright install chromium && npx playwright test

# A release of the device (like MacDub's): VERSION=0.1.0 make release
# builds every platform, packs them in dist/release/v<version>/ with
# SHA256SUMS, updates Casks/go-link-console.rb and creates the GitHub release with
# gh (a pre-release while the macOS app is not signed and notarized).
release:
	./scripts/release.sh

# macOS installer: the universal go-link-console.app in a drag-to-Applications disk
# image (dist/device/go-link-<version>-macos-universal.dmg), in the go-link
# style. With CODESIGN_IDENTITY="Developer ID Application: ..." the image
# is signed.
device-dmg: device-darwin-universal
	CODESIGN_IDENTITY="$(CODESIGN_IDENTITY)" $(DEVICE_DIR)/build/macos/make-dmg.sh $(DEVICE_OUT)/darwin-universal/go-link-console.app \
		$(DEVICE_OUT)/go-link-$(VERSION)-macos-universal.dmg

# The local web panel of headless devices: the rooms' website (play), built
# and copied into the device, which embeds it (backend-device/web). `make
# panel` refreshes it; the device builds make it when it is missing.
PANEL_DIST = $(DEVICE_DIR)/web/panel/dist

panel: web-build
	rm -rf $(PANEL_DIST) && mkdir -p $(PANEL_DIST) && cp -R $(PLAY_DIST)/. $(PANEL_DIST)/
	@echo "$(GREEN)✓ Web panel copied into $(PANEL_DIST)$(NC)"

$(PANEL_DIST)/index.html:
	@$(MAKE) panel

device: $(if $(filter darwin,$(HOST_OS)),device-darwin-universal) \
	device-linux-amd64 device-linux-arm64 device-linux-amd64-headless device-linux-arm64-headless \
	device-windows-amd64 device-windows-arm64

# macOS ships as an app bundle (go-link-console.app): opened from the Finder it
# runs like any Mac app, without a Terminal window; from a terminal,
# go-link-console.app/Contents/MacOS/go-link-console takes the flags and
# subcommands. The release is one universal app (Intel + Apple silicon),
# made on either kind of Mac: clang builds both architectures, and
# build/macos/static-libs.sh builds libvpx and libopus from source for
# each one and for macOS $(MIN_MACOS), linked statically (Homebrew's copies
# only fit this Mac's CPU and macOS version). It is signed ad hoc
# (codesign -s -); to hand it to other people it needs a Developer ID
# signature and notarization. -Wl,-w hides the linker's notes about the
# assembler objects (no platform load command), which are harmless.
MIN_MACOS      = 12.0
MACOS_LIBS     = $(DIST)/.macos-libs
MACOS_CLANG    = $$(xcrun -f clang)
clang_arch     = $(if $(filter amd64,$(1)),x86_64,arm64)

# go-link HD's engine (its own repository, github.com/lordbasex/golink-hd,
# next to this one) ships inside every build: go-link-console.app's Frameworks,
# beside the binary in the Linux and Windows archives, in the Docker image.
# It is built from the repository's committed HEAD (git archive), never
# from files left in its folder.
GOLINK_HD_DIR ?= ../golink-hd
GOLINK_HD_SRC  = $(DIST)/.golinkhd/src

golinkhd-src:
	@test -d $(GOLINK_HD_DIR)/.git || { echo "$(RED)✗ go-link HD's engine is not in $(GOLINK_HD_DIR) (git clone https://github.com/lordbasex/golink-hd there, or set GOLINK_HD_DIR)$(NC)"; exit 1; }
	@rm -rf $(GOLINK_HD_SRC) && mkdir -p $(GOLINK_HD_SRC)
	@git -C $(GOLINK_HD_DIR) archive HEAD | tar -x -C $(GOLINK_HD_SRC)
	@echo "$(GREEN)✓ go-link HD $$(git -C $(GOLINK_HD_DIR) describe --tags --always) goes in the build$(NC)"

# The engine library of one Mac architecture, for macOS $(MIN_MACOS)+.
$(DEVICE_OUT)/darwin-%/libgolinkhd.dylib: golinkhd-src
	@mkdir -p $(DEVICE_OUT)/darwin-$*
	@$(MAKE) -s -B -C $(GOLINK_HD_SRC) platform=osx arch=$(call clang_arch,$*) CC="$(MACOS_CLANG)" \
		MACOSX_DEPLOYMENT_TARGET=$(MIN_MACOS) SDKROOT="$$(xcrun --sdk macosx --show-sdk-path)"
	cp $(GOLINK_HD_SRC)/libgolinkhd.dylib $@

# The device binary of one architecture: dist/device/darwin-<arch>/go-link-console.
$(DEVICE_OUT)/darwin-%/go-link-console: $(PANEL_DIST)/index.html FORCE
	@if [ "$(HOST_OS)" != "darwin" ]; then echo "$(RED)✗ macOS builds need a Mac$(NC)"; exit 1; fi
	@echo "$(YELLOW)Building the device for darwin/$* (macOS $(MIN_MACOS)+)...$(NC)"
	@$(DEVICE_DIR)/build/macos/static-libs.sh $* $(MACOS_LIBS)/$*
	@mkdir -p $(DEVICE_OUT)/darwin-$*
	cd $(DEVICE_DIR) && GOOS=darwin GOARCH=$* CGO_ENABLED=1 \
		CC="$(MACOS_CLANG) -arch $(call clang_arch,$*)" \
		MACOSX_DEPLOYMENT_TARGET=$(MIN_MACOS) SDKROOT="$$(xcrun --sdk macosx --show-sdk-path)" \
		PKG_CONFIG_LIBDIR=$(MACOS_LIBS)/$*/lib/pkgconfig \
		CGO_CFLAGS="-O2 -mmacosx-version-min=$(MIN_MACOS)" \
		CGO_LDFLAGS="-mmacosx-version-min=$(MIN_MACOS) -Wl,-w" \
		go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
		-o $(DEVICE_OUT)/darwin-$*/go-link-console ./cmd/device

# One architecture's app, for development: dist/device/darwin-<arch>/go-link-console.app.
device-darwin-%: $(DEVICE_OUT)/darwin-%/go-link-console $(DEVICE_OUT)/darwin-%/libgolinkhd.dylib
	@$(DEVICE_DIR)/build/macos/make-app.sh $(DEVICE_OUT)/darwin-$*/go-link-console \
		$(DEVICE_OUT)/darwin-$*/go-link-console.app $(VERSION) $(SHORT_VERSION) "$(CODESIGN_IDENTITY)" \
		$(DEVICE_OUT)/darwin-$*/libgolinkhd.dylib

# The release app: both architectures in one binary (lipo),
# dist/device/darwin-universal/go-link-console.app.
device-darwin-universal: $(DEVICE_OUT)/darwin-amd64/go-link-console $(DEVICE_OUT)/darwin-arm64/go-link-console \
		$(DEVICE_OUT)/darwin-amd64/libgolinkhd.dylib $(DEVICE_OUT)/darwin-arm64/libgolinkhd.dylib
	@mkdir -p $(DEVICE_OUT)/darwin-universal
	lipo -create -output $(DEVICE_OUT)/darwin-universal/go-link-console $(DEVICE_OUT)/darwin-amd64/go-link-console $(DEVICE_OUT)/darwin-arm64/go-link-console
	lipo -create -output $(DEVICE_OUT)/darwin-universal/libgolinkhd.dylib $(DEVICE_OUT)/darwin-amd64/libgolinkhd.dylib $(DEVICE_OUT)/darwin-arm64/libgolinkhd.dylib
	@$(DEVICE_DIR)/build/macos/make-app.sh $(DEVICE_OUT)/darwin-universal/go-link-console \
		$(DEVICE_OUT)/darwin-universal/go-link-console.app $(VERSION) $(SHORT_VERSION) "$(CODESIGN_IDENTITY)" \
		$(DEVICE_OUT)/darwin-universal/libgolinkhd.dylib

FORCE:

# Linux builds in Docker (the other architecture through QEMU).
device-linux-%-headless: $(PANEL_DIST)/index.html golinkhd-src
	@echo "$(YELLOW)Building the headless device for linux/$*...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build --platform linux/$* --build-arg TAGS=headless --build-arg VERSION=$(VERSION) \
		--build-context golinkhd=$(GOLINK_HD_SRC) \
		-f build/linux.Dockerfile --output type=local,dest=$(DEVICE_OUT)/linux-$*-headless .
	@echo "$(GREEN)✓ $(DEVICE_OUT)/linux-$*-headless/go-link-console$(NC)"

device-linux-%: $(PANEL_DIST)/index.html golinkhd-src
	@echo "$(YELLOW)Building the device for linux/$*...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build --platform linux/$* --build-arg VERSION=$(VERSION) \
		--build-context golinkhd=$(GOLINK_HD_SRC) \
		-f build/linux.Dockerfile --output type=local,dest=$(DEVICE_OUT)/linux-$* .
	@echo "$(GREEN)✓ $(DEVICE_OUT)/linux-$*/go-link-console$(NC)"

# Windows cross-compiles in Docker with llvm-mingw and static libraries.
device-windows-amd64:
	@$(MAKE) device-windows MINGW_ARCH=x86_64 GO_ARCH=amd64

device-windows-arm64:
	@$(MAKE) device-windows MINGW_ARCH=aarch64 GO_ARCH=arm64

device-windows: $(PANEL_DIST)/index.html golinkhd-src
	@echo "$(YELLOW)Building the device for windows/$(GO_ARCH)...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build --build-arg ARCH=$(MINGW_ARCH) --build-arg GOARCH=$(GO_ARCH) \
		--build-context golinkhd=$(GOLINK_HD_SRC) \
		--build-arg VERSION=$(VERSION) -f build/windows.Dockerfile \
		--output type=local,dest=$(DEVICE_OUT)/windows-$(GO_ARCH) .
	@echo "$(GREEN)✓ $(DEVICE_OUT)/windows-$(GO_ARCH)/go-link-console.exe$(NC)"

# ---------------------------------------------------------------------------
# The licenses that travel with every binary and image.
LEGAL_DIR = $(DIST)/.legal
legal:
	@mkdir -p $(LEGAL_DIR) && cp LICENSE THIRD_PARTY_NOTICES.md $(LEGAL_DIR)/

# The device as a container (headless, web panel on 7373): loaded into the
# local Docker for this computer's CPU, and an OCI archive for amd64 and
# arm64 (Raspberry Pi). Not pushed to any registry.
device-docker: $(PANEL_DIST)/index.html legal golinkhd-src
	@echo "$(YELLOW)Building the go-link-console image...$(NC)"
	cd $(DEVICE_DIR) && docker buildx build -f build/docker.Dockerfile --build-arg VERSION=$(VERSION) \
		--build-context legal=$(LEGAL_DIR) --build-context golinkhd=$(GOLINK_HD_SRC) -t go-link-console:$(VERSION) -t go-link-console:latest --load .
	@echo "$(GREEN)✓ go-link-console:$(VERSION) (docker compose -f $(DEVICE_DIR)/docker-compose.yml up -d)$(NC)"

device-docker-oci: $(PANEL_DIST)/index.html legal golinkhd-src
	mkdir -p $(DOCKER_OUT)
	cd $(DEVICE_DIR) && docker buildx build -f build/docker.Dockerfile --build-arg VERSION=$(VERSION) \
		--build-context legal=$(LEGAL_DIR) --build-context golinkhd=$(GOLINK_HD_SRC) --platform linux/amd64,linux/arm64 -t go-link-console:$(VERSION) \
		--output type=oci,dest=$(DOCKER_OUT)/go-link-console-$(VERSION).oci.tar .
	@echo "$(GREEN)✓ $(DOCKER_OUT)/go-link-console-$(VERSION).oci.tar$(NC)"

# ---------------------------------------------------------------------------

clean:
	rm -rf $(DIST) $(SITE_DIST) $(PLAY_DIST) $(MAKER_DIST)
	@echo "$(GREEN)✓ Clean$(NC)"

# The Android player app (mobile/android). Signing comes from
# mobile/android/keystore.properties, the properties file ANDROID_SIGNING
# names (kept outside the repository, e.g. set in deploy/local/hosting.mk) or
# GOLINK_* environment variables. android-apk refuses an unsigned APK.
ANDROID_DIR := mobile/android
ANDROID_SIGNING ?=
# versionCode grows with every release: 0.1.4 -> 104, 1.2.3 -> 10203.
ANDROID_VERSION_CODE = $(shell echo "$(VERSION)" | sed 's/^v//' | awk -F. '{print $$1*10000 + $$2*100 + $$3}')
# Gradle needs JDK 17: Homebrew's openjdk@17 when present (CI sets its own).
ANDROID_JAVA_HOME ?= $(firstword $(wildcard /usr/local/opt/openjdk@17 /opt/homebrew/opt/openjdk@17))
ANDROID_ENV = $(if $(ANDROID_JAVA_HOME),JAVA_HOME=$(ANDROID_JAVA_HOME))
APKSIGNER = $(lastword $(sort $(wildcard $(HOME)/Library/Android/sdk/build-tools/*/apksigner)))

android-debug:
	cd $(ANDROID_DIR) && $(ANDROID_ENV) ./gradlew :core:test :app:assembleDebug

android-apk:
	@test -n "$(VERSION)" || { echo "Set VERSION, e.g. VERSION=0.1.4"; exit 1; }
	rm -rf $(ANDROID_DIR)/app/build/outputs/apk/release
	cd $(ANDROID_DIR) && $(ANDROID_ENV) GOLINK_SIGNING="$(ANDROID_SIGNING)" ./gradlew :core:test :app:assembleRelease \
	  -PversionName=$(patsubst v%,%,$(VERSION)) -PversionCode=$(ANDROID_VERSION_CODE)
	mkdir -p dist/android
	@f=$$(ls $(ANDROID_DIR)/app/build/outputs/apk/release/*.apk | head -1); \
	  case "$$f" in *unsigned*) echo "The APK is unsigned: set ANDROID_SIGNING (see mobile/android/README.md)"; exit 1;; esac; \
	  if [ -n "$(APKSIGNER)" ]; then "$(APKSIGNER)" verify --print-certs "$$f" | grep "SHA-256" || exit 1; fi; \
	  cp "$$f" dist/android/go-link-player-v$(patsubst v%,%,$(VERSION)).apk; \
	  cp "$$f" dist/android/go-link-player.apk; \
	  echo "$(GREEN)dist/android/go-link-player-v$(patsubst v%,%,$(VERSION)).apk$(NC) (from $$f)"

help:
	@echo "$(GREEN)go-link $(VERSION)$(NC)"
	@echo ""
	@echo "$(YELLOW)Everything:$(NC)"
	@echo "  make all                     web + device (every platform)"
	@echo ""
	@echo "$(YELLOW)Website:$(NC)"
	@echo "  make web-build               build frontend/apps/web/dist-site and dist-play"
	@echo "  make web-deploy              build and upload (deploy/local/hosting.mk)"
	@echo "  make maker-build             build Willy Maker's site, frontend/willy-maker/dist"
	@echo "  make maker-deploy            build and upload it (deploy/local/hosting.mk)"
	@echo ""
	@echo "$(YELLOW)Device:$(NC)"
	@echo "  make device                  every platform into dist/device/"
	@echo "  make device-darwin-universal the macOS app for Intel + Apple silicon (macOS $(MIN_MACOS)+, on any Mac)"
	@echo "  make device-darwin-amd64     one architecture only (also -darwin-arm64), for development"
	@echo "  make device-linux-amd64      also: -linux-arm64, -linux-amd64-headless, -linux-arm64-headless (Docker)"
	@echo "  make device-windows-amd64    also: -windows-arm64 (Docker, llvm-mingw)"
	@echo "  make e2e                     end-to-end tests in a real browser (own signaling server, device, web)"
	@echo "  make device-dmg              the macOS app in a drag-to-Applications .dmg"
	@echo "  VERSION=0.1.0 make release   every platform packed in dist/release/ + GitHub release (gh)"
	@echo "  make panel                   rebuild the web panel that headless devices serve"
	@echo "  make device-docker           the device as a Docker image (headless, panel on :7373)"
	@echo "  make device-docker-oci       the image for amd64 + arm64, as an OCI archive"
	@echo ""
	@echo "$(YELLOW)Android app (go-link Console Player, needs JDK 17 + Android SDK):$(NC)"
	@echo "  make android-debug           core tests + debug APK (mobile/android)"
	@echo "  VERSION=0.1.0 make android-apk  signed release APK in dist/android/ (see mobile/android/README.md)"
	@echo ""
	@$(MAKE) --no-print-directory hosting-help
	@echo ""
	@echo "  make info | clean"
