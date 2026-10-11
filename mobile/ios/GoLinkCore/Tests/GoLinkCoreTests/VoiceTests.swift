// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

final class VoiceTests: XCTestCase {
    private let slots: [VoiceSlot?] = [
        VoiceSlot(name: "Ana", port: 2, you: false),
        nil,
        VoiceSlot(name: "Cy", port: 0, you: false),
        VoiceSlot(name: "Bo", port: 1, you: false),
    ]

    func testSlotsLightTheSpeakersSeatOrQueueName() {
        let s = VoiceSlots.speaking(slots: slots, levels: [1: 0.2, 2: 0.9, 3: 0.1, 4: 0.01], silenced: [])
        // Slot 2 is empty and slot 4 is too quiet.
        XCTAssertEqual(Speaking(ports: [2], names: ["Cy"]), s)
        // Someone silenced never shows as speaking.
        XCTAssertEqual(Speaking(ports: [], names: ["Cy"]), VoiceSlots.speaking(slots: slots, levels: [1: 0.2, 3: 0.1], silenced: ["Ana"]))
        XCTAssertEqual(Speaking(), VoiceSlots.speaking(slots: [nil, nil, nil, nil], levels: [1: 1], silenced: []))
    }

    func testSilencingGoesByName() {
        XCTAssertEqual([3], VoiceSlots.mutedSlots(slots: slots, silenced: ["Cy", "Nobody"]))
        XCTAssertEqual([1, 4], VoiceSlots.mutedSlots(slots: slots, silenced: ["Ana", "Bo"]))
        // The same person moved to another slot: that slot is muted now.
        let moved: [VoiceSlot?] = [nil, VoiceSlot(name: "Ana", port: 2, you: false), nil, nil]
        XCTAssertEqual([2], VoiceSlots.mutedSlots(slots: moved, silenced: ["Ana"]))
    }

    func testTheVoiceQuestion() {
        var ask = VoiceAsk()
        // Watching: nothing to ask.
        XCTAssertFalse(ask.update(talker: false, roomVoice: true, micOn: false))
        // Joins the queue: asked once.
        XCTAssertTrue(ask.update(talker: true, roomVoice: true, micOn: false))
        XCTAssertTrue(ask.update(talker: true, roomVoice: true, micOn: false))
        ask.dismiss() // Not now
        XCTAssertFalse(ask.update(talker: true, roomVoice: true, micOn: false))
        // Back to watching, then a seat: asked again.
        XCTAssertFalse(ask.update(talker: false, roomVoice: true, micOn: false))
        XCTAssertTrue(ask.update(talker: true, roomVoice: true, micOn: false))
        // Stops being able to talk: the question goes away.
        XCTAssertFalse(ask.update(talker: false, roomVoice: true, micOn: false))
        // The microphone already on, or a room without voice: never asked.
        XCTAssertFalse(ask.update(talker: true, roomVoice: true, micOn: true))
        var other = VoiceAsk()
        XCTAssertFalse(other.update(talker: true, roomVoice: false, micOn: false))
        // The microphone turned on while asked: it goes away.
        var third = VoiceAsk()
        XCTAssertTrue(third.update(talker: true, roomVoice: true, micOn: false))
        XCTAssertFalse(third.update(talker: true, roomVoice: true, micOn: true))
    }
}
