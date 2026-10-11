// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// Wire format of the signalhub protocol v1. signalhub's README (its own
// repository) is the contract; this mirrors frontend/packages/shared/src/protocol.ts
// and the Android app's Protocol.kt.

public enum GoLinkProtocol {
    public static let version = "1"

    /** signalhub namespace of this project (its ALLOWED_APPS). */
    public static let app = "go-link-console"

    /** The official signaling server. STUN/TURN are never hardcoded: they come in hello. */
    public static let officialSignalURL = "wss://signal.go-link.org/ws"
}

/** Fixed error texts sent by signalhub, so clients can compare them. */
public enum ServerError {
    public static let invalidCode = "invalid or expired code"
    public static let appNotAllowed = "app not allowed"
    public static let roomNotFound = "room not found"
    public static let invalidInvite = "invalid or expired invite"
    public static let rateLimited = "rate limit exceeded"
    public static let tooManyMessages = "too many messages"
    public static let deviceOffline = "device not connected"
}

/** Adds ?v=1 unless the URL already names a protocol version, like the web's endpoint(). */
public func endpoint(_ url: String) -> String {
    let parts = url.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)
    let query = parts.count > 1 ? String(parts[1]) : ""
    if query.split(separator: "&", omittingEmptySubsequences: false).contains(where: { $0 == "v" || $0.hasPrefix("v=") }) {
        return url
    }
    return parts.count > 1 ? "\(url)&v=\(GoLinkProtocol.version)" : "\(url)?v=\(GoLinkProtocol.version)"
}

/** Messages the app sends to signalhub (envelopes). */
public enum Envelopes {
    /** join through an invitation link (invite) or a typed 9-digit code. */
    public static func join(_ target: InviteTarget) -> JSON {
        switch target {
        case let .link(invite): return .obj(("type", "join"), ("app", .string(GoLinkProtocol.app)), ("invite", .string(invite)))
        case let .code(code): return .obj(("type", "join"), ("app", .string(GoLinkProtocol.app)), ("code", .string(code)))
        }
    }

    /** A signal to the host device: payload is opaque to signalhub. */
    public static func signal(to: String, payload: JSON) -> JSON {
        .obj(("type", "signal"), ("to", .string(to)), ("payload", payload))
    }
}

/** The envelope fields the client reads. */
public struct Envelope: Equatable, Sendable {
    public let type: String
    public let raw: JSONObject

    public var error: String { raw["error"].str(200) }
    public var from: String { raw["from"].str(64) }
    public var remote: String { raw["remote"].str(64) }
    public var roomId: String { raw["room_id"].str(64) }
    public var sessionId: String { raw["session_id"].str(64) }
    public var peerId: String { raw["peer_id"].str(64) }
    public var payload: JSON? { raw["payload"] }

    public static func parse(_ text: String) -> Envelope? {
        guard let o = JSONText.parseObject(text), case let .string(type)? = o["type"] else { return nil }
        return Envelope(type: String(type.prefix(40)), raw: o)
    }
}
