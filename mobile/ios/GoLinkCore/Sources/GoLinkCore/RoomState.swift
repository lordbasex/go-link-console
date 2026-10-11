// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// Room state and chat sent by the device's Room Manager on the "control"
// DataChannel. It comes from the host, but names and chat are typed by
// other guests, so everything is validated and shown as plain text.
// Port of frontend/packages/shared/src/room-state.ts and the Android app's
// RoomState.kt.

public struct SeatView: Equatable, Sendable {
    public let port: Int
    public let name: String
    public let localPlayer: Int
    public let you: Bool
    /** The player's peer id: sent only to the host's own browsers ("" otherwise). */
    public var peer = ""

    public init(port: Int, name: String, localPlayer: Int, you: Bool, peer: String = "") {
        self.port = port
        self.name = name
        self.localPlayer = localPlayer
        self.you = you
        self.peer = peer
    }
}

public struct QueueEntry: Equatable, Sendable {
    public let position: Int
    public let name: String
    public let you: Bool
    /** Sent only to the host's own browsers ("" otherwise). */
    public var peer = ""

    public init(position: Int, name: String, you: Bool, peer: String = "") {
        self.position = position
        self.name = name
        self.you = you
        self.peer = peer
    }
}

public struct Spectator: Equatable, Sendable {
    public let name: String
    public let you: Bool
    /** Sent only to the host's own browsers ("" otherwise). */
    public var peer = ""
    /** A raised hand: this spectator asks the host to play. */
    public var hand = false

    public init(name: String, you: Bool, peer: String = "", hand: Bool = false) {
        self.name = name
        self.you = you
        self.peer = peer
        self.hand = hand
    }
}

/** A person speaking on one of the four voice tracks (voice-p1..voice-p4). */
public struct VoiceSlot: Equatable, Sendable {
    public let name: String
    /** Their seat, or 0 when they wait in the queue. */
    public let port: Int
    public let you: Bool

    public init(name: String, port: Int, you: Bool) {
        self.name = name
        self.port = port
        self.you = you
    }
}

/** A request to swap controllers: from port asks to move to port to. */
public struct SwapView: Equatable, Sendable {
    public let from: Int
    public let to: Int
    public let name: String

    public init(from: Int, to: Int, name: String) {
        self.from = from
        self.to = to
        self.name = name
    }
}

/** What the device tells guests about the room. art is base64 JPEG, or nil. */
public struct RoomInfoView: Equatable, Sendable {
    public let title: String
    public let game: String
    public let host: String
    public let artBase64: String?

    public init(title: String, game: String, host: String, artBase64: String?) {
        self.title = title
        self.game = game
        self.host = host
        self.artBase64 = artBase64
    }
}

public struct GameControls: Equatable, Sendable {
    /** Players the game takes at once, 1 to 4 (0 = unknown). */
    public let players: Int
    /** Action buttons per player, 0 to 6. */
    public let buttons: Int
    /** MAME control type: joy4way, joy8way, stick, dial... ("" = unknown). */
    public let control: String

    public init(players: Int, buttons: Int, control: String) {
        self.players = players
        self.buttons = buttons
        self.control = control
    }

    /** What an unknown game (or an older device) offers. */
    public static let `default` = GameControls(players: 0, buttons: 6, control: "joy8way")
}

public struct YouView: Equatable, Sendable {
    public let name: String
    public let ports: [Int]
    public let queuePositions: [Int]
    public let spectator: Bool
    public let swapOffers: [SwapView]
    public let swapAsked: [SwapView]
    /** Your pending request for a pause (only the host pauses), or nil. */
    public var pauseAsked: PauseAsk?
    /** Your hand is up (a spectator asking the host to play). */
    public var hand = false
    /** When you may raise your hand again (ms since the epoch), or 0. */
    public var handAgain: Double = 0
    /** The host moved you to watching: you raise a hand instead of joining the queue alone. */
    public var held = false
}

/** A pause this guest asked the host for; expiresAt is the device's RFC 3339 time ("" if not sent). */
public struct PauseAsk: Equatable, Sendable {
    public let expiresAt: String

    public init(expiresAt: String) { self.expiresAt = expiresAt }
}

/** Where you stand: seated, waiting in the queue, or watching. */
public enum Me: Equatable, Sendable {
    case player([Int])
    case queue(Int)
    case spectator
}

public struct RoomStateView: Equatable, Sendable {
    public let maxPlayers: Int
    public let voice: Bool
    public let chat: Bool
    public let info: RoomInfoView
    /** Index 0 is P1; nil means a free seat. */
    public let seats: [SeatView?]
    public let queue: [QueueEntry]
    public let spectators: [Spectator]
    public let you: YouView
    public let pausable: Bool
    public let paused: Bool
    public let pausedBy: String
    public let controls: GameControls
    public let recording: Bool
    /**
     * Whether the host (the device's owner) has a browser in the room to
     * answer a pause request. Older devices do not send it: true.
     */
    public var hostOnline: Bool = true
    /** The host's default picture for this room (picture: {style, bands}); nil = the app's default. */
    public var picture: PictureSettings?
    /** Spectators hear the voices too (the host's switch). */
    public var spectatorsHear = false
    /** Spectators get the lighter video (the host's switch). */
    public var spectatorSaver = false
    /** Who each voice track (voice-p1..voice-p4) carries now, always 4 entries; nil: nobody. */
    public var voiceSlots: [VoiceSlot?] = [nil, nil, nil, nil]

    public var me: Me {
        if !you.ports.isEmpty { return .player(you.ports) }
        if let first = you.queuePositions.min() { return .queue(first) }
        return .spectator
    }

    public var seated: Int { seats.filter { $0 != nil }.count }

    /** The players and the queue talk (the device drops a spectator's voice). */
    public var talker: Bool {
        switch me {
        case .player, .queue: return true
        case .spectator: return false
        }
    }

    /** The ports this player holds (empty when not seated). */
    public var myPorts: [Int] {
        if case let .player(p) = me { return p }
        return []
    }
}

/** System chat lines the app shows in the reader's language. */
public enum ChatEvent: String, CaseIterable, Sendable {
    case recordingStarted = "recording_started"
    case recordingStopped = "recording_stopped"
    case gamePaused = "game_paused"
    case gameResumed = "game_resumed"
    case nowWatching = "now_watching"
    case moved
    case swapAsked = "swap_asked"
    case keptSeat = "kept_seat"
    case swapped
    case leftSeat = "left_seat"
    case seatFree = "seat_free"
    case tookSeat = "took_seat"
    case sentToQueue = "sent_to_queue"
    case sentToWatch = "sent_to_watch"
    /** The host (name) let a spectator (name2) into the queue. */
    case letQueue = "let_queue"
    /** The host (name) seated name2 at port. */
    case playNow = "play_now"
    case firstInQueue = "first_in_queue"
    /** The host (name) took name2 out of the room. */
    case removed
    /** Sent only to the guest whose pause request the host declined. */
    case pauseDeclined = "pause_declined"
    /** Sent only to the spectator whose raised hand the host answered "not now". */
    case handLater = "hand_later"
}

/** The values of a chat event (who and which seat). */
public struct ChatArgs: Equatable, Sendable {
    public let name: String
    public let port: Int
    public let name2: String
    public let port2: Int

    public init(name: String, port: Int, name2: String, port2: Int) {
        self.name = name
        self.port = port
        self.name2 = name2
        self.port2 = port2
    }
}

public enum ChatLine: Equatable, Sendable {
    case system(text: String, ts: Double, event: ChatEvent? = nil, args: ChatArgs? = nil)
    case user(name: String, port: Int?, role: String, text: String, ts: Double)

    public var ts: Double {
        switch self {
        case let .system(_, ts, _, _): return ts
        case let .user(_, _, _, _, ts): return ts
        }
    }
}

/** Someone else typing a chat message. */
public struct TypingView: Equatable, Sendable {
    public let name: String
    public let port: Int?

    public init(name: String, port: Int?) {
        self.name = name
        self.port = port
    }
}

/** Frames per second the device sends, the picture's display aspect and how it is sent (2x or not). */
public struct StreamStatsView: Equatable, Sendable {
    public let fps: Double?
    public let aspect: Double?
    /** stream_stats "video" (nil: older devices, or before the first frame, so scale 1). */
    public let video: StreamVideo?

    public init(fps: Double?, aspect: Double?, video: StreamVideo? = nil) {
        self.fps = fps
        self.aspect = aspect
        self.video = video
    }
}

public enum RoomMessages {
    private static let base64 = try! NSRegularExpression(pattern: "^[A-Za-z0-9+/]+={0,2}$")
    private static let guest = try! NSRegularExpression(pattern: "^Guest ([0-9A-F]{1,8})$")

    private static func parseControls(_ v: JSON?) -> GameControls {
        guard case let .object(o)? = v else { return .default }
        let buttons = o["buttons"].isNumber ? jsRound(o["buttons"].num).clamped(0, 6) : 6
        let players = o["players"].isNumber ? jsRound(o["players"].num).clamped(0, 4) : 0
        return GameControls(players: players, buttons: buttons, control: o["control"].str(20))
    }

    private static func parseInfo(_ v: JSON?) -> RoomInfoView {
        let o = v.obj
        var art = o["art"].strOrNil
        if let a = art, a.count > 6000 || !Invites.matches(base64, a) { art = nil }
        return RoomInfoView(title: o["title"].str(80), game: o["game"].str(160), host: o["host"].str(60), artBase64: art)
    }

    private static func port(_ v: JSON?, max: Int) -> Int {
        let n = v.num
        return n == n.rounded(.down) && n >= 1 && n <= Double(max) ? Int(n) : 0
    }

    private static func parseSwaps(_ v: JSON?, maxPlayers: Int) -> [SwapView] {
        v.arr.prefix(8).compactMap { x in
            let o = Optional(x).obj
            let from = port(o["from"], max: maxPlayers)
            let to = port(o["to"], max: maxPlayers)
            return from != 0 && to != 0 && from != to ? SwapView(from: from, to: to, name: o["name"].str(40)) : nil
        }
    }

    private static func parsePauseAsk(_ v: JSON?) -> PauseAsk? {
        guard case let .object(o)? = v else { return nil }
        let at = o["expires_at"]
        return PauseAsk(expiresAt: at.isNumber ? String(jsRound(at.num)) : at.str(40))
    }

    public static func parseRoomState(_ m: JSONObject) -> RoomStateView? {
        guard m["type"].str(40) == "room_state" else { return nil }
        let maxPlayers = Int(m["max_players"].num.clamped(1, 4))
        var seats = [SeatView?](repeating: nil, count: maxPlayers)
        for (i, s) in m["seats"].arr.enumerated() {
            guard i < maxPlayers, case let .object(o) = s else { continue }
            seats[i] = SeatView(port: i + 1, name: o["name"].str(40), localPlayer: Int(o["local_player"].num), you: o["you"].isTrue, peer: o["peer"].str(80))
        }
        let you = m["you"].obj
        return RoomStateView(
            maxPlayers: maxPlayers,
            voice: !m["voice"].isFalse,
            chat: !m["chat"].isFalse,
            info: parseInfo(m["info"]),
            seats: seats,
            queue: m["queue"].arr.prefix(64).map { q in
                let o = Optional(q).obj
                return QueueEntry(position: Int(o["position"].num), name: o["name"].str(40), you: o["you"].isTrue, peer: o["peer"].str(80))
            },
            spectators: m["spectators"].arr.prefix(64).map { p in
                let o = Optional(p).obj
                return Spectator(name: o["name"].str(40), you: o["you"].isTrue, peer: o["peer"].str(80), hand: o["hand"].isTrue)
            },
            you: YouView(
                name: you["name"].str(40),
                ports: you["ports"].arr.prefix(4).map { Int(Optional($0).num) },
                queuePositions: you["queue_positions"].arr.prefix(4).map { Int(Optional($0).num) },
                spectator: you["spectator"].isTrue,
                swapOffers: parseSwaps(you["swap_offers"], maxPlayers: maxPlayers),
                swapAsked: parseSwaps(you["swap_asked"], maxPlayers: maxPlayers),
                pauseAsked: parsePauseAsk(you["pause_asked"]),
                hand: you["hand"].isTrue,
                handAgain: parseTime(you["hand_again"]),
                held: you["held"].isTrue
            ),
            pausable: m["pausable"].isTrue,
            paused: m["paused"].isTrue,
            pausedBy: m["paused_by"].str(40),
            controls: parseControls(m["controls"]),
            recording: m["recording"].isTrue,
            hostOnline: !m["host_online"].isFalse,
            picture: parsePicture(m["picture"]),
            spectatorsHear: m["spectators_hear"].isTrue,
            spectatorSaver: m["spectator_saver"].isTrue,
            voiceSlots: parseVoiceSlots(m["voice_slots"], maxPlayers: maxPlayers)
        )
    }

    /** room_state.voice_slots: always 4 entries; a port outside the seats counts as the queue (0). */
    static func parseVoiceSlots(_ v: JSON?, maxPlayers: Int) -> [VoiceSlot?] {
        let list = v.arr
        return (0..<4).map { i in
            guard i < list.count, case let .object(o) = list[i] else { return nil }
            return VoiceSlot(name: o["name"].str(40), port: port(o["port"], max: maxPlayers), you: o["you"].isTrue)
        }
    }

    private static let rfc3339 = try! NSRegularExpression(pattern: "^(\\d{4}-\\d\\d-\\d\\dT\\d\\d:\\d\\d:\\d\\d)(\\.\\d{1,9})?(Z|[+-]\\d\\d:\\d\\d)$")

    /**
     * An RFC 3339 time from the device (Go writes up to 9 decimals) as ms
     * since the epoch, or 0 when absent or not a time (the website's time()).
     */
    public static func parseTime(_ v: JSON?) -> Double {
        guard let s = v.strOrNil, s.count <= 40,
              let m = rfc3339.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)),
              let base = Range(m.range(at: 1), in: s), let zone = Range(m.range(at: 3), in: s)
        else { return 0 }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        guard let date = f.date(from: String(s[base]) + String(s[zone])) else { return 0 }
        var ms = date.timeIntervalSince1970 * 1000
        if let frac = Range(m.range(at: 2), in: s), let x = Double("0" + s[frac]) { ms += x * 1000 }
        return ms.rounded(.down)
    }

    /** room_state.picture: nil when absent or when either value is unknown (the website's parseRoomPicture). */
    public static func parsePicture(_ v: JSON?) -> PictureSettings? {
        guard case .object? = v else { return nil }
        let o = v.obj
        guard let style = o["style"].strOrNil.flatMap(PictureStyle.init(rawValue:)),
              let bands = o["bands"].strOrNil.flatMap(PictureBands.init(rawValue:)) else { return nil }
        return PictureSettings(style: style, bands: bands)
    }

    public static func parseChat(_ m: JSONObject) -> ChatLine? {
        guard m["type"].str(40) == "chat" else { return nil }
        let ts = m["ts"].num
        if let system = m["system"].strOrNil {
            let text = String(system.prefix(300))
            guard let wire = m["event"].strOrNil, let event = ChatEvent(rawValue: wire) else {
                return .system(text: text, ts: ts)
            }
            guard let rawArgs = m["args"] else { return .system(text: text, ts: ts, event: event) }
            let a = Optional(rawArgs).obj
            return .system(
                text: text, ts: ts, event: event,
                args: ChatArgs(name: a["name"].str(60), port: port(a["port"], max: 4), name2: a["name2"].str(60), port2: port(a["port2"], max: 4))
            )
        }
        guard let text = m["text"].strOrNil, let name = m["name"].strOrNil else { return nil }
        let p = m["port"].num
        return .user(name: String(name.prefix(40)), port: (1...4).contains(p) ? Int(p) : nil, role: m["role"].str(20), text: String(text.prefix(300)), ts: ts)
    }

    /** Who is typing when m is a "typing" message. */
    public static func parseTyping(_ m: JSONObject) -> [TypingView]? {
        guard m["type"].str(40) == "typing" else { return nil }
        return m["names"].arr.prefix(20).map { x in
            let o = Optional(x).obj
            let p = o["port"].num
            return TypingView(name: o["name"].str(40), port: (1...4).contains(p) ? Int(p) : nil)
        }.filter { !$0.name.isEmpty }
    }

    public static func parseStreamStats(_ m: JSONObject) -> StreamStatsView? {
        guard m["type"].str(40) == "stream_stats" else { return nil }
        let fps = m["fps"].isNumber ? m["fps"].num : nil
        var aspect: Double?
        if m["aspect"].isNumber {
            let a = m["aspect"].num
            if a > 0.2 && a < 5 { aspect = a }
        }
        return StreamStatsView(fps: fps, aspect: aspect, video: StreamVideo.parse(m))
    }

    /**
     * The id of a name the device generated ("Guest 9F3A" -> "9F3A"), so
     * the app can show it in the reader's language; nil for real names.
     */
    public static func guestId(_ name: String) -> String? { Invites.firstGroup(guest, name) }
}
