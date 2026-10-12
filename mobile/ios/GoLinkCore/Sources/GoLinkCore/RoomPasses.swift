// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/** Small string storage the platform provides (UserDefaults on iOS). */
public protocol KeyValueStore: AnyObject {
    func get(_ key: String) -> String?
    func set(_ key: String, _ value: String?)
}

/** A store in memory, for tests and previews. */
public final class MemoryStore: KeyValueStore {
    public var map: [String: String] = [:]

    public init() {}

    public func get(_ key: String) -> String? { map[key] }

    public func set(_ key: String, _ value: String?) { map[key] = value }
}

/**
 * The return token the device gives on the way into a room, kept per
 * room_id like the web's localStorage "go-link.room-passes". It is only
 * used to get back in automatically after a dropped connection in the same
 * visit; a new join always asks for the invitation's PIN. Room ids change
 * with every session of a room, so only the last few are kept.
 */
public final class RoomPasses {
    public static let key = "go-link.room-passes"
    public static let max = 20

    private let store: KeyValueStore

    public init(_ store: KeyValueStore) { self.store = store }

    public func get(_ roomId: String) -> String {
        read().first(where: { $0.0 == roomId })?.1 ?? ""
    }

    public func save(_ roomId: String, _ token: String) {
        var passes = read().filter { $0.0 != roomId }
        passes.append((roomId, token)) // newest last
        while passes.count > Self.max { passes.removeFirst() }
        write(passes)
    }

    public func forget(_ roomId: String) {
        let passes = read()
        let kept = passes.filter { $0.0 != roomId }
        if kept.count != passes.count { write(kept) }
    }

    /** Entries in saved order (oldest first). */
    private func read() -> [(String, String)] {
        // Stored as an array of pairs, so the order survives the round trip.
        guard let text = store.get(Self.key), let json = JSONText.parse(text) else { return [] }
        switch json {
        case let .array(items):
            return items.compactMap { item in
                guard case let .array(pair) = item, pair.count == 2,
                      case let .string(k) = pair[0], case let .string(v) = pair[1] else { return nil }
                return (k, v)
            }
        case let .object(o):
            // The web's shape (an object); order unknown.
            return o.pairs.compactMap { k, v in
                if case let .string(s) = v { return (k, s) }
                return nil
            }
        default:
            return []
        }
    }

    private func write(_ passes: [(String, String)]) {
        store.set(Self.key, JSON.array(passes.map { .array([.string($0.0), .string($0.1)]) }).text)
    }
}

/** The terms of use every player accepts before joining (docs/legal.md). */
public enum Terms {
    /** Must equal TERMS_VERSION in frontend/apps/web/src/legal.ts (a test checks it). */
    public static let version = "2026-10-11"
    public static let termsURL = "https://console.go-link.org/terms"
    public static let privacyURL = "https://console.go-link.org/privacy"
    public static let storeKey = "go-link.terms"

    public static func accepted(_ store: KeyValueStore) -> Bool {
        guard let text = store.get(storeKey), let o = JSONText.parseObject(text) else { return false }
        return o["version"].strOrNil == version
    }

    public static func accept(_ store: KeyValueStore, isoNow: String) {
        store.set(storeKey, JSON.obj(("version", .string(version)), ("at", .string(isoNow))).text)
    }
}
