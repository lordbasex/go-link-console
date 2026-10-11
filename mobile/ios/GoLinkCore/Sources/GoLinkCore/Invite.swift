// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/**
 * What an invitation points at: the 22-character invite of a
 * https://go-link.org/g/<invite> link (and its QR code), or the room's
 * 9-digit code. The PIN is never part of either: the person types it.
 */
public enum InviteTarget: Equatable, Hashable, Sendable {
    case link(String)
    case code(String)

    /** How it is shown: the link, or the code grouped like the web shows it ("123 456 789"). */
    public var display: String {
        switch self {
        case let .link(invite): return "https://\(Invites.host)/g/\(invite)"
        case let .code(c):
            let a = Array(c)
            guard a.count == 9 else { return c }
            return "\(String(a[0..<3])) \(String(a[3..<6])) \(String(a[6...]))"
        }
    }
}

/**
 * An invitation as the app opens it: where it points, and the key of a
 * group invitation (one link for several people, `#k=<key>` after the
 * link) or "" for a one-person invitation. The key only lives in memory:
 * it is never logged or saved, and description leaves it out.
 */
public struct Invitation: Equatable, Hashable, Sendable, CustomStringConvertible {
    public let target: InviteTarget
    public let key: String

    public init(target: InviteTarget, key: String = "") {
        self.target = target
        self.key = key
    }

    /** A group invitation: the person sends a name instead of a PIN. */
    public var isGroup: Bool { !key.isEmpty }

    public var description: String { "Invitation(\(target.display)\(isGroup ? ", group" : ""))" }
}

public enum Invites {
    /** The only site whose invitation links the app follows. */
    public static let host = "go-link.org"

    /** Largest text worth looking at: a QR code can carry kilobytes. */
    static let maxInput = 200

    private static let invite = try! NSRegularExpression(pattern: "^[A-Za-z0-9_-]{22}$")
    private static let link = try! NSRegularExpression(pattern: "^https://go-link\\.org/g/([A-Za-z0-9_-]{22})/?$")
    /** The same link with a group invitation's key: exactly #k=<43 base64url characters>. */
    private static let groupLink = try! NSRegularExpression(pattern: "^https://go-link\\.org/g/([A-Za-z0-9_-]{22})/?#k=([A-Za-z0-9_-]{43})$")
    private static let fragmentKey = try! NSRegularExpression(pattern: "(?:^|[#&])k=([A-Za-z0-9_-]{43})(?:&|$)")
    private static let path = try! NSRegularExpression(pattern: "^/g/([A-Za-z0-9_-]{22})/?$")
    private static let code = try! NSRegularExpression(pattern: "^[0-9]{9}$")
    private static let separators = try! NSRegularExpression(pattern: "[\\s.-]")

    /**
     * Reads a scanned QR code. Only two things are accepted: exactly an
     * https://go-link.org/g/<invite> link, or a 9-digit code. Anything
     * else (another site, a signaling server, a PIN, parameters) is
     * rejected, so a hostile QR code can never point the app elsewhere.
     */
    public static func parseScanned(_ text: String) -> InviteTarget? {
        if text.count > maxInput { return nil }
        let t = trim(text)
        if let m = firstGroup(link, t) { return .link(m) }
        return parseCode(t)
    }

    /**
     * Reads what a person typed or pasted: the link (as strict as a QR
     * code), the bare 22-character invite, or the 9-digit code (spaces,
     * dots and dashes allowed, as the web shows it: "123 456 789").
     */
    public static func parseTyped(_ text: String) -> InviteTarget? {
        if text.count > maxInput { return nil }
        let t = trim(text)
        if let m = firstGroup(link, t) { return .link(m) }
        if matches(invite, t) { return .link(t) }
        return parseCode(t)
    }

    /**
     * Reads a Universal Link: scheme https, host go-link.org, path
     * /g/<invite>. Query and fragment are ignored, never used.
     */
    public static func parseAppLink(scheme: String?, host: String?, path: String?) -> InviteTarget? {
        guard scheme == "https", host == Self.host, let path else { return nil }
        guard let m = firstGroup(Self.path, path) else { return nil }
        return .link(m)
    }

    /** Reads a Universal Link from its URL (user info and port are refused). */
    public static func parseAppLink(_ url: URL) -> InviteTarget? {
        guard let c = URLComponents(url: url, resolvingAgainstBaseURL: false),
              c.user == nil, c.password == nil, c.port == nil
        else { return nil }
        return parseAppLink(scheme: c.scheme?.lowercased(), host: c.host?.lowercased(), path: c.percentEncodedPath)
    }

    /**
     * Reads a scanned QR code like parseScanned, also taking a group
     * invitation's link (https://go-link.org/g/<invite>#k=<key>).
     */
    public static func scanned(_ text: String) -> Invitation? {
        if text.count > maxInput { return nil }
        let t = trim(text)
        if let g = groupOf(t) { return g }
        return parseScanned(t).map { Invitation(target: $0) }
    }

    /** Reads typed or pasted text like parseTyped, also taking a group invitation's link. */
    public static func typed(_ text: String) -> Invitation? {
        if text.count > maxInput { return nil }
        let t = trim(text)
        if let g = groupOf(t) { return g }
        return parseTyped(t).map { Invitation(target: $0) }
    }

    /**
     * Reads a Universal Link like parseAppLink, plus a group invitation's
     * key from its fragment (k=<key>; anything else there is ignored).
     */
    public static func appLink(_ url: URL) -> Invitation? {
        guard let target = parseAppLink(url) else { return nil }
        let fragment = URLComponents(url: url, resolvingAgainstBaseURL: false)?.percentEncodedFragment ?? ""
        return Invitation(target: target, key: groupKey(fragment: fragment))
    }

    /**
     * The key of a group invitation from a link's fragment ("k=<key>",
     * maybe among other values joined by &), or "" when there is none or
     * it is not exactly 43 base64url characters. A "#" in front is allowed.
     */
    public static func groupKey(fragment: String) -> String {
        if fragment.count > maxInput { return "" }
        return firstGroup(fragmentKey, fragment) ?? ""
    }

    private static func groupOf(_ t: String) -> Invitation? {
        guard let m = groupLink.firstMatch(in: t, range: NSRange(t.startIndex..., in: t)), m.numberOfRanges > 2,
              let a = Range(m.range(at: 1), in: t), let b = Range(m.range(at: 2), in: t)
        else { return nil }
        return Invitation(target: .link(String(t[a])), key: String(t[b]))
    }

    /** A PIN is exactly 6 digits. */
    public static func isPin(_ pin: String) -> Bool {
        pin.count == 6 && pin.allSatisfy { ("0"..."9").contains($0) }
    }

    private static func parseCode(_ t: String) -> InviteTarget? {
        let digits = separators.stringByReplacingMatches(in: t, range: NSRange(t.startIndex..., in: t), withTemplate: "")
        return matches(code, digits) ? .code(digits) : nil
    }

    private static func trim(_ s: String) -> String {
        s.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    static func matches(_ re: NSRegularExpression, _ s: String) -> Bool {
        re.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) != nil
    }

    static func firstGroup(_ re: NSRegularExpression, _ s: String) -> String? {
        guard let m = re.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)), m.numberOfRanges > 1,
              let r = Range(m.range(at: 1), in: s)
        else { return nil }
        return String(s[r])
    }
}
