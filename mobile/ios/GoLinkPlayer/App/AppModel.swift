// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import GoLinkCore
import SwiftUI

/** The app's screens. */
enum Screen: Equatable {
    case home
    case scan
    /**
     * The PIN form (or the name form of a group invitation). The invitation
     * is fixed when it came from a QR code or a link, or nil to type the
     * code too. A group invitation's key stays in memory only.
     */
    case join(Invitation?)
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
        // (-invite https://go-link.org/g/<invite>), since Universal Links
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
     * A Universal Link (https://go-link.org/g/<invite>) goes straight to the
     * PIN form for that invitation. Only the invitation (and a group
     * invitation's key, #k=<key>) is read from the link: never a PIN or a
     * signaling server.
     */
    func open(url: URL) {
        guard let invitation = Invites.appLink(url) else { return }
        openInvite(invitation)
    }

    /** An invitation from a QR code or a link: straight to the PIN (or the name, for a group invitation). */
    func openInvite(_ invitation: Invitation) {
        leaveRoom()
        screen = .join(invitation)
    }

    var termsAccepted: Bool { Terms.accepted(prefs) }

    /**
     * Joins with the typed PIN, or a group invitation's key with the name
     * the person gave (saved as the player name), after the terms were
     * accepted on the form.
     */
    func join(_ invitation: Invitation, pin: String, name: String = "") {
        Terms.accept(prefs, isoNow: ISO8601DateFormatter().string(from: Date()))
        if invitation.isGroup { prefs.playerName = name }
        leaveRoom()
        let s = RoomSession(prefs: prefs, invitation: invitation, pin: pin, name: name)
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
