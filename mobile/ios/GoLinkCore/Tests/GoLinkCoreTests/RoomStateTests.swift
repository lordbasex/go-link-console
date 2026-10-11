// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

final class RoomStateTests: XCTestCase {
    func testParsesARoomState() {
        let m = obj("""
        {"type":"room_state","max_players":2,"voice":true,"chat":false,
        "info":{"title":"Friday","game":"Some Game","host":"Fede","art":"QUJD"},
        "seats":[{"name":"Ana","local_player":0,"you":true},null,{"name":"extra"}],
        "queue":[{"position":1,"name":"Guest 9F3A","you":false}],
        "spectators":[{"name":"Bo","you":false}],
        "you":{"name":"Ana","ports":[1],"queue_positions":[],"spectator":false,
          "swap_offers":[{"from":2,"to":1,"name":"Bo"},{"from":1,"to":1},{"from":3,"to":1}],"swap_asked":[]},
        "pausable":true,"paused":true,"paused_by":"Ana",
        "controls":{"players":2,"buttons":9,"control":"joy4way"},"recording":true}
        """)
        let s = RoomMessages.parseRoomState(m)!
        XCTAssertEqual(2, s.maxPlayers)
        XCTAssertFalse(s.chat)
        XCTAssertEqual([SeatView(port: 1, name: "Ana", localPlayer: 0, you: true), nil], s.seats)
        XCTAssertEqual(RoomInfoView(title: "Friday", game: "Some Game", host: "Fede", artBase64: "QUJD"), s.info)
        XCTAssertEqual([SwapView(from: 2, to: 1, name: "Bo")], s.you.swapOffers)
        XCTAssertEqual(GameControls(players: 2, buttons: 6, control: "joy4way"), s.controls)
        XCTAssertEqual(.player([1]), s.me)
        XCTAssertTrue(s.paused && s.pausable && s.recording)
        XCTAssertEqual("9F3A", RoomMessages.guestId(s.queue[0].name))
        XCTAssertNil(RoomMessages.guestId("Ana"))
    }

    func testDefaultsForOlderDevices() {
        let s = RoomMessages.parseRoomState(obj(#"{"type":"room_state","you":{"queue_positions":[3,2]}}"#))!
        XCTAssertEqual(1, s.maxPlayers)
        XCTAssertTrue(s.voice)
        XCTAssertTrue(s.chat)
        XCTAssertEqual(GameControls.default, s.controls)
        XCTAssertEqual(.queue(2), s.me)
        XCTAssertNil(RoomMessages.parseRoomState(obj(#"{"type":"chat"}"#)))
    }

    func testChatLines() {
        let user = RoomMessages.parseChat(obj(#"{"type":"chat","name":"Ana","port":1,"role":"player","text":"hi","ts":5}"#))
        XCTAssertEqual(.user(name: "Ana", port: 1, role: "player", text: "hi", ts: 5), user)
        let sys = RoomMessages.parseChat(obj(#"{"type":"chat","system":"Ana took seat P2","event":"took_seat","args":{"name":"Ana","port":2},"ts":1}"#))
        XCTAssertEqual(.system(text: "Ana took seat P2", ts: 1, event: .tookSeat, args: ChatArgs(name: "Ana", port: 2, name2: "", port2: 0)), sys)
        let unknown = RoomMessages.parseChat(obj(#"{"type":"chat","system":"new thing","event":"future_event"}"#))
        XCTAssertEqual(.system(text: "new thing", ts: 0), unknown)
        XCTAssertNil(RoomMessages.parseChat(obj(#"{"type":"chat","text":"no name"}"#)))
    }

    func testTypingAndStats() {
        let who = RoomMessages.parseTyping(obj(#"{"type":"typing","names":[{"name":"Bo","port":2},{"name":""}]}"#))
        XCTAssertEqual([TypingView(name: "Bo", port: 2)], who)
        let st = RoomMessages.parseStreamStats(obj(#"{"type":"stream_stats","fps":59.9,"aspect":1.3333}"#))
        XCTAssertEqual(StreamStatsView(fps: 59.9, aspect: 1.3333), st)
        XCTAssertEqual(StreamStatsView(fps: nil, aspect: nil), RoomMessages.parseStreamStats(obj(#"{"type":"stream_stats","aspect":9}"#)))
    }

    func testHandsVoiceSlotsAndHostFields() {
        let s = RoomMessages.parseRoomState(obj("""
        {"type":"room_state","max_players":2,
        "seats":[{"name":"Ana","you":false,"peer":"p-ana"},{"name":"Bo","you":false}],
        "queue":[{"position":1,"name":"Cy","you":false,"peer":"p-cy"}],
        "spectators":[{"name":"Di","you":true,"hand":true,"hand_at":"2026-10-11T10:00:00Z"},{"name":"Ed","you":false}],
        "you":{"name":"Di","hand":true,"hand_again":"2026-10-11T10:01:00.123456789-03:00","held":true},
        "spectators_hear":true,"spectator_saver":true,
        "voice_slots":[{"name":"Ana","port":1,"you":false},null,{"name":"Cy","port":0},{"name":"Zed","port":3}]}
        """))!
        XCTAssertEqual("p-ana", s.seats[0]?.peer)
        XCTAssertEqual("", s.seats[1]?.peer)
        XCTAssertEqual(QueueEntry(position: 1, name: "Cy", you: false, peer: "p-cy"), s.queue[0])
        XCTAssertEqual([Spectator(name: "Di", you: true, hand: true), Spectator(name: "Ed", you: false)], s.spectators)
        XCTAssertTrue(s.you.hand)
        XCTAssertTrue(s.you.held)
        // 13:01:00.123 UTC.
        XCTAssertEqual(1_791_723_660_123, s.you.handAgain)
        XCTAssertEqual(.spectator, s.me)
        XCTAssertFalse(s.talker)
        XCTAssertTrue(s.spectatorsHear && s.spectatorSaver)
        // Always four slots; a port outside the seats counts as the queue.
        XCTAssertEqual([
            VoiceSlot(name: "Ana", port: 1, you: false), nil,
            VoiceSlot(name: "Cy", port: 0, you: false), VoiceSlot(name: "Zed", port: 0, you: false),
        ], s.voiceSlots)

        let old = RoomMessages.parseRoomState(obj(#"{"type":"room_state","you":{"queue_positions":[1],"hand_again":"soon"}}"#))!
        XCTAssertEqual([nil, nil, nil, nil], old.voiceSlots)
        XCTAssertFalse(old.you.hand || old.you.held || old.spectatorsHear || old.spectatorSaver)
        XCTAssertEqual(0, old.you.handAgain)
        XCTAssertTrue(old.talker) // the queue talks
    }

    func testHandAndPromotionChatEvents() {
        let cases: [(String, ChatEvent)] = [
            ("let_queue", .letQueue), ("play_now", .playNow), ("first_in_queue", .firstInQueue),
            ("removed", .removed), ("hand_later", .handLater), ("sent_to_queue", .sentToQueue), ("sent_to_watch", .sentToWatch),
        ]
        for (wire, event) in cases {
            let line = RoomMessages.parseChat(obj(#"{"type":"chat","system":"x","event":"\#(wire)","args":{"name":"Host","name2":"Di","port":2},"ts":1}"#))
            XCTAssertEqual(.system(text: "x", ts: 1, event: event, args: ChatArgs(name: "Host", port: 2, name2: "Di", port2: 0)), line, wire)
        }
    }

    func testPauseRequestFields() {
        let s = RoomMessages.parseRoomState(obj(#"{"type":"room_state","host_online":false,"you":{"pause_asked":{"expires_at":"2026-09-29T10:00:30Z"}}}"#))!
        XCTAssertFalse(s.hostOnline)
        XCTAssertEqual(PauseAsk(expiresAt: "2026-09-29T10:00:30Z"), s.you.pauseAsked)
        let none = RoomMessages.parseRoomState(obj(#"{"type":"room_state","host_online":true,"you":{"pause_asked":null}}"#))!
        XCTAssertTrue(none.hostOnline)
        XCTAssertNil(none.you.pauseAsked)
        // Older devices send neither: the host counts as online.
        XCTAssertTrue(RoomMessages.parseRoomState(obj(#"{"type":"room_state"}"#))!.hostOnline)
        let declined = RoomMessages.parseChat(obj(#"{"type":"chat","system":"The host declined","event":"pause_declined","ts":3}"#))
        XCTAssertEqual(.system(text: "The host declined", ts: 3, event: .pauseDeclined), declined)
    }
}
