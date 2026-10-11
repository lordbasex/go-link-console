// swift-tools-version:5.10
// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import PackageDescription

// The portable logic of go-link Console Player for iOS: a port of the Android
// app's :core module (mobile/android/core). No UIKit and no WebRTC here,
// so it builds and tests on the Mac with `swift test`.
let package = Package(
    name: "GoLinkCore",
    platforms: [.iOS(.v17), .macOS(.v14)],
    products: [
        .library(name: "GoLinkCore", targets: ["GoLinkCore"]),
    ],
    targets: [
        .target(name: "GoLinkCore"),
        .testTarget(name: "GoLinkCoreTests", dependencies: ["GoLinkCore"]),
    ]
)
