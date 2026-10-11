# Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
# go-link Console Player for iOS. `make help` lists the targets.

SIM ?= iPhone 17
DEST ?= platform=iOS Simulator,name=$(SIM)
# VERSION=0.1.5 sets the app's version (MARKETING_VERSION) and its build
# number (CURRENT_PROJECT_VERSION) with the Android app's version code rule:
# major*10000 + minor*100 + patch (0.1.5 -> 105). Without it, the latest
# release tag (v0.1.9 -> 0.1.9), else Config/Base.xcconfig.
VERSION ?= $(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null | sed 's/^v//')
ifneq ($(VERSION),)
VERSION_CODE := $(shell echo '$(VERSION)' | awk -F. '{print $$1*10000 + $$2*100 + $$3}')
VERSION_FLAGS := MARKETING_VERSION=$(VERSION) CURRENT_PROJECT_VERSION=$(VERSION_CODE)
endif
XCB = xcodebuild -project GoLinkPlayer.xcodeproj -scheme GoLinkPlayer -clonedSourcePackagesDirPath build/spm -derivedDataPath build/dd $(VERSION_FLAGS)

# Signing for a device or TestFlight, all from the environment (never from
# the repository): `source ~/.go-link-signing/env.sh` sets TEAM_ID and the App
# Store Connect API key (ASC_KEY_ID, ASC_ISSUER_ID, ASC_KEY_PATH). With the key
# xcodebuild creates the certificates and profiles, registers the iPhone and
# uploads, without Xcode's Accounts.
TEAM_FLAG := $(if $(TEAM_ID),DEVELOPMENT_TEAM=$(TEAM_ID))
ASC_AUTH := -allowProvisioningUpdates $(if $(ASC_KEY_ID),-authenticationKeyPath "$(ASC_KEY_PATH)" -authenticationKeyID $(ASC_KEY_ID) -authenticationKeyIssuerID $(ASC_ISSUER_ID))
# TestFlight needs a new build number on every upload: the version code and
# the time (0.3.0 at 12:30 on 2026-10-09 -> 300.2610091230).
BUILD ?= $(or $(VERSION_CODE),1).$(shell date +%y%m%d%H%M)
# The connected iPhone (the first one paired), for install-device.
DEVICE ?= $(shell xcrun devicectl list devices 2>/dev/null | grep 'available (paired)' | grep -m1 iPhone | grep -oE '[0-9A-F]{8}-([0-9A-F]{4}-){3}[0-9A-F]{12}')

.PHONY: help project test-core build-sim test-ui device-build install-device testflight clean

help: ## This list
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-13s %s\n", $$1, $$2}'

project: ## Generate GoLinkPlayer.xcodeproj from project.yml (needs xcodegen)
	xcodegen generate

test-core: ## GoLinkCore unit tests on the Mac
	cd GoLinkCore && swift test

build-sim: project ## Debug build for the iOS Simulator (no signing)
	$(XCB) -destination '$(DEST)' CODE_SIGNING_ALLOWED=NO build

test-ui: project ## UI tests in the Simulator (screenshots with TEST_RUNNER_GL_SHOTS=dir)
	$(XCB) -destination '$(DEST)' test

device-build: project ## Signed build for a connected iPhone (team in Config/Local.xcconfig)
	$(XCB) -destination 'generic/platform=iOS' -allowProvisioningUpdates build

install-device: project ## Build, sign and install on the connected iPhone (source env.sh first)
	@test -n "$(DEVICE)" || { echo "no paired iPhone connected" >&2; exit 1; }
	$(XCB) -configuration Debug -destination 'generic/platform=iOS' $(TEAM_FLAG) $(ASC_AUTH) build
	xcrun devicectl device install app --device $(DEVICE) build/dd/Build/Products/Debug-iphoneos/GoLinkPlayer.app
	xcrun devicectl device process launch --device $(DEVICE) org.golink.player

testflight: project ## Archive, sign and upload a build to App Store Connect / TestFlight (source env.sh first)
	@test -n "$(TEAM_ID)" -a -n "$(ASC_KEY_ID)" || { echo "source ~/.go-link-signing/env.sh first (TEAM_ID, ASC_KEY_ID)" >&2; exit 1; }
	rm -rf build/GoLinkPlayer.xcarchive build/export
	$(XCB) -configuration Release -destination 'generic/platform=iOS' $(TEAM_FLAG) CURRENT_PROJECT_VERSION=$(BUILD) \
		$(ASC_AUTH) -archivePath build/GoLinkPlayer.xcarchive archive
	printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0"><dict>' \
		'<key>method</key><string>app-store-connect</string>' \
		'<key>destination</key><string>upload</string>' \
		'<key>signingStyle</key><string>automatic</string>' \
		'<key>teamID</key><string>$(TEAM_ID)</string>' \
		'<key>uploadSymbols</key><true/>' \
		'<key>manageAppVersionAndBuildNumber</key><false/>' \
		'</dict></plist>' > build/ExportOptions.plist
	xcodebuild -exportArchive -archivePath build/GoLinkPlayer.xcarchive -exportPath build/export \
		-exportOptionsPlist build/ExportOptions.plist $(ASC_AUTH)
	@echo "uploaded build $(BUILD): App Store Connect processes it in a few minutes, then it shows in TestFlight"

clean: ## Remove build output and the generated project
	rm -rf build GoLinkPlayer.xcodeproj
