// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// What travels inside signalhub's "signal" payload between the app and the
// device (docs/protocol.md "Negotiation" and "The room PIN gate"). The
// device always offers; the app answers. signalhub never reads it.

/** An ICE candidate as the web's RTCIceCandidateInit. */
public struct IceCandidateInit: Equatable, Sendable {
    public let candidate: String
    public let sdpMid: String?
    public let sdpMLineIndex: Int

    public init(candidate: String, sdpMid: String?, sdpMLineIndex: Int) {
        self.candidate = candidate
        self.sdpMid = sdpMid
        self.sdpMLineIndex = sdpMLineIndex
    }
}

/**
 * The answer to a PIN or token. reason is "wrong" (left tries remain),
 * "used" (someone already came in with that invitation), "blocked" (no
 * tries left), "locked" (too many wrong PINs in the room: wait
 * retryAfter seconds) or "removed" (the host took this guest out of the
 * room: nothing brings it back but a new invitation). A group
 * invitation's key also gets "waiting" (the host decides), "declined",
 * "full", "expired" or "busy". token lets this app come back without a PIN.
 */
public struct PinResult: Equatable, Sendable {
    public let ok: Bool
    public let token: String
    public let reason: String
    public let left: Int
    public let retryAfter: Int

    public init(ok: Bool, token: String, reason: String, left: Int, retryAfter: Int) {
        self.ok = ok
        self.token = token
        self.reason = reason
        self.left = left
        self.retryAfter = retryAfter
    }
}

public enum DeviceSignal: Equatable, Sendable {
    case offer(String)
    case candidate(IceCandidateInit)
    /** The room asks for the PIN of an invitation before streaming anything. */
    case pinRequired
    case pinResult(PinResult)
}

public enum RtcSignals {
    private static let token = try! NSRegularExpression(pattern: "^[A-Za-z0-9_-]{43}$")
    private static let voice = try! NSRegularExpression(pattern: "^voice-p([1-4])$")

    /** A return token or owner key: 43 base64url characters, else "". */
    public static func tokenOf(_ v: JSON?) -> String {
        guard let s = v.strOrNil, Invites.matches(token, s) else { return "" }
        return s
    }

    public static func parse(_ payload: JSON?) -> DeviceSignal? {
        guard case let .object(o)? = payload else { return nil }
        switch o["kind"].str(20) {
        case "offer":
            guard let sdp = o["sdp"].strOrNil, !sdp.isEmpty else { return nil }
            return .offer(sdp)
        case "candidate":
            guard case let .object(c)? = o["candidate"], let text = c["candidate"].strOrNil else { return nil }
            return .candidate(IceCandidateInit(
                candidate: String(text.prefix(1000)),
                sdpMid: c["sdpMid"].strOrNil.map { String($0.prefix(32)) },
                sdpMLineIndex: c["sdpMLineIndex"].isNumber ? Int(c["sdpMLineIndex"].num) : 0
            ))
        case "pin_required":
            return .pinRequired
        case "pin_result":
            func n(_ v: JSON?) -> Int { v.isNumber ? max(0, jsRound(v.num)) : 0 }
            return .pinResult(PinResult(
                ok: o["ok"].isTrue,
                token: tokenOf(o["token"]),
                reason: o["reason"].str(20),
                left: n(o["left"]),
                retryAfter: n(o["retry_after"])
            ))
        default:
            return nil
        }
    }

    public static func answer(_ sdp: String) -> JSON { .obj(("kind", "answer"), ("sdp", .string(sdp))) }

    public static func candidate(_ c: IceCandidateInit) -> JSON {
        .obj(("kind", "candidate"), ("candidate", .obj(
            ("candidate", .string(c.candidate)),
            ("sdpMid", c.sdpMid.map { .string($0) }),
            ("sdpMLineIndex", .int(c.sdpMLineIndex))
        )))
    }

    /** The PIN of an invitation, as the person typed it. */
    public static func pin(_ pin: String) -> JSON { .obj(("kind", "pin"), ("pin", .string(pin))) }

    /** Most characters of the name sent with a group invitation's key. */
    public static let keyNameMax = 24

    /**
     * A group invitation's key and the person's name, which the host sees
     * while deciding (24 characters at most).
     */
    public static func key(_ key: String, name: String) -> JSON {
        .obj(("kind", "pin"), ("key", .string(key)), ("name", .string(String(name.prefix(keyNameMax)))))
    }

    /** The return token got on the way in, to come back without a PIN. */
    public static func token(_ token: String) -> JSON { .obj(("kind", "pin"), ("token", .string(token))) }

    /**
     * mid of the audio m-line the device offers as recvonly: the
     * microphone. The app answers it sendonly (see docs/protocol.md).
     */
    public static func micMid(_ sdp: String) -> String? {
        let normalized = sdp.replacingOccurrences(of: "\r\n", with: "\n")
        let sections = normalized.components(separatedBy: "\nm=").dropFirst()
        for section in sections {
            guard section.hasPrefix("audio") else { continue }
            let lines = section.split(separator: "\n").map(String.init)
            guard lines.contains("a=recvonly") else { continue }
            if let mid = lines.first(where: { $0.hasPrefix("a=mid:") })?.dropFirst(6), !mid.isEmpty {
                return String(mid)
            }
        }
        return nil
    }

    /**
     * Which voice slot (1-4) a voice track is, from its stream id
     * ("voice-p2" -> 2). room_state.voice_slots says who each one carries.
     */
    public static func voicePort(_ streamId: String) -> Int? { Invites.firstGroup(voice, streamId).flatMap(Int.init) }
}
