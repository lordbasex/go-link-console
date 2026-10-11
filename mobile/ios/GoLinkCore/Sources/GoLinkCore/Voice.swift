// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

// The room's voice, as the website's RoomPage does it: the players and the
// queue talk (the device drops a spectator's voice), and the four incoming
// voice tracks (voice-p1..voice-p4) are slots that carry whoever speaks
// now, named by room_state.voice_slots.

/** Who is speaking right now: seats (ports) and, for the queue, names. */
public struct Speaking: Equatable, Sendable {
    public var ports: Set<Int> = []
    public var names: Set<String> = []

    public init(ports: Set<Int> = [], names: Set<String> = []) {
        self.ports = ports
        self.names = names
    }
}

public enum VoiceSlots {
    /** A voice track's level (0 to 1, linear like WebRTC's audioLevel) above which someone speaks. */
    public static let speakingLevel = 0.03

    /**
     * Who speaks, from each slot's level (slot 1-4 -> 0..1): a player
     * lights up their seat, someone in the queue their name. Silenced
     * people never show as speaking.
     */
    public static func speaking(slots: [VoiceSlot?], levels: [Int: Double], silenced: Set<String>) -> Speaking {
        var out = Speaking()
        for (i, who) in slots.prefix(4).enumerated() {
            guard let who, (levels[i + 1] ?? 0) > speakingLevel, !silenced.contains(who.name) else { continue }
            if who.port > 0 { out.ports.insert(who.port) } else { out.names.insert(who.name) }
        }
        return out
    }

    /**
     * The slots (1-4) to play muted: those carrying someone this listener
     * silenced. A slot is not a person, so silencing goes by name.
     */
    public static func mutedSlots(slots: [VoiceSlot?], silenced: Set<String>) -> Set<Int> {
        var out = Set<Int>()
        for (i, who) in slots.prefix(4).enumerated() {
            if let who, silenced.contains(who.name) { out.insert(i + 1) }
        }
        return out
    }
}

/**
 * "Do you want to join the voice?": asked each time this app becomes able
 * to talk (it joins as a player or in the queue, or is moved there from
 * watching) while its microphone is off and the room has voice. It goes
 * away when the person stops being able to talk, the room's voice goes off
 * or the microphone turns on.
 */
public struct VoiceAsk: Equatable, Sendable {
    public private(set) var shown = false
    private var wasTalker = false

    public init() {}

    /** Feeds the room's latest state; returns whether the question shows. */
    @discardableResult
    public mutating func update(talker: Bool, roomVoice: Bool, micOn: Bool) -> Bool {
        if talker && !wasTalker && roomVoice && !micOn { shown = true }
        if !talker || !roomVoice || micOn { shown = false }
        wasTalker = talker
        return shown
    }

    /** "Not now", or the microphone was turned on from the question. */
    public mutating func dismiss() { shown = false }
}
