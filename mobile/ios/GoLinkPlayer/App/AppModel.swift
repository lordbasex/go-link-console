// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import GoLinkCore
import SwiftUI

/** The app's screens. */
enum Screen: Equatable {
    case home
    case scan
    /** The PIN form. The target is fixed when it came from a QR code or a link, or nil to type the code too. */
    case join(InviteTarget?)
    case room
    case settings
    /** The offline controller test (no device, no network). */
    case testController
}

@MainActor
final class AppModel: ObservableObject {
    let prefs = Prefs()
    @Published var screen: Screen = .home
    @Published private(set) var signal: SignalUrls.Choice
    @Published private(set) var session: RoomSession?

    init() {
        #if DEBUG
        // Screenshot runs start like a first launch.
        if ProcessInfo.processInfo.arguments.contains("-resetState") {
            for key in [Terms.storeKey, Prefs.signalKey, Prefs.nameKey, RoomPasses.key] { prefs.set(key, nil) }
        }
        #endif
        signal = prefs.signal
        #if DEBUG
        // Debug builds take an invitation from the launch arguments
        // (-invite https://play.go-link.org/g/<invite>), since Universal Links
        // need the published association file and a signed app. The same
        // strict parser is used, so only the invitation is read.
        let args = ProcessInfo.processInfo.arguments
        if let i = args.firstIndex(of: "-invite"), i + 1 < args.count, let url = URL(string: args[i + 1]) {
            open(url: url)
        }
        if let i = args.firstIndex(of: "-signal"), i + 1 < args.count, case let .ok(url) = SignalUrls.check(args[i + 1], allowDevHosts: true) {
            setSignal(url)
        }
        // Automated tests on a desk: measure the played sound, but play silence.
        if args.contains("-silentOutput") { WebRtcEngine.shared.audioDevice.silentOutput = true }
        #endif
    }

    /**
     * A Universal Link (https://play.go-link.org/g/<invite>) goes straight to the
     * PIN form for that invitation. Only the invitation is read from the
     * link: never a PIN or a signaling server.
     */
    func open(url: URL) {
        guard let target = Invites.parseAppLink(url) else { return }
        openInvite(target)
    }

    /** An invitation from a QR code or a link: straight to the PIN. */
    func openInvite(_ target: InviteTarget) {
        leaveRoom()
        screen = .join(target)
    }

    var termsAccepted: Bool { Terms.accepted(prefs) }

    /** Joins with the typed PIN, after the terms were accepted on the form. */
    func join(_ target: InviteTarget, pin: String) {
        Terms.accept(prefs, isoNow: ISO8601DateFormatter().string(from: Date()))
        leaveRoom()
        let s = RoomSession(prefs: prefs, target: target, pin: pin)
        session = s
        s.start()
        screen = .room
    }

    func leaveRoom() {
        session?.close()
        session = nil
        if screen == .room { screen = .home }
    }

    func setName(_ name: String) {
        prefs.playerName = name
        session?.setName(prefs.playerName)
    }

    /** Saves a custom signaling server (already tested), or nil for the official one. */
    func setSignal(_ url: String?) {
        prefs.saveCustomSignal(url)
        signal = prefs.signal
    }
}
