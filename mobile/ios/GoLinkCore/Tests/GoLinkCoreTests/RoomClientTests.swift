// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

@MainActor
final class RoomClientTests: XCTestCase {
    private let invite = "AbCdEfGhIjKlMnOpQr_-12"
    private let token = String(repeating: "t", count: 43)

    private func payload(_ m: JSONObject) -> String { m["payload"]!.text }

    private func kinds(_ c: FakeSockets.Conn) -> [String] {
        c.sent.filter { $0["type"] == .string("signal") }.map { $0["payload"]?["kind"].str() ?? "" }
    }

    func testJoinsSendsThePinAnswersAndComesBackWithTheToken() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let store = MemoryStore()
        let peers = FakePeers()
        let signal = SignalClient(url: "wss://signal.example.org/ws", factory: sockets, scheduler: clock, random: { 0.5 })
        let room = RoomClient(signal: signal, target: .link(invite), typedPin: "482913", passes: RoomPasses(store), peers: peers, scheduler: clock)
        room.start()
        let first = sockets.last
        XCTAssertEqual("wss://signal.example.org/ws?v=1", first.url)
        first.receive(#"{"type":"hello","peer_id":"me1","ice_servers":[{"urls":["stun:signal.example.org:3478"]}]}"#)
        XCTAssertEqual([#"{"type":"join","app":"go-link","invite":"\#(invite)"}"#], first.sentText)

        // The device asks for the PIN right after the join: the typed PIN goes out.
        first.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}"#)
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
        XCTAssertEqual(.pin, room.ui.phase)
        XCTAssertEqual(#"{"kind":"pin","pin":"482913"}"#, payload(first.sent.last!))
        XCTAssertEqual("dev", first.sent.last!["to"].str())

        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":true,"token":"\#(token)"}}"#)
        XCTAssertEqual(.connecting, room.ui.phase)
        XCTAssertEqual(token, RoomPasses(store).get("room-1"))

        // Offer, early candidate, answer; a signal from someone else is ignored.
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"candidate","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}}"#)
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"offer","sdp":"o1"}}"#)
        first.receive(#"{"type":"signal","from":"intruder","payload":{"kind":"offer","sdp":"evil"}}"#)
        XCTAssertEqual(1, peers.created.count)
        let peer = peers.created[0]
        XCTAssertEqual([IceServer(urls: ["stun:signal.example.org:3478"])], peers.servers[0])
        XCTAssertEqual(["o1"], peer.offers)
        XCTAssertEqual([IceCandidateInit(candidate: "c1", sdpMid: "0", sdpMLineIndex: 0)], peer.candidates)
        XCTAssertEqual(#"{"kind":"answer","sdp":"answer-for-o1"}"#, payload(first.sent.last!))

        // The control channel opens: hello with the name and local players; pings are answered.
        room.setIdentity(name: "Ana", localPlayers: [1, 0, 1])
        peer.events.onControlOpen()
        XCTAssertEqual(#"{"type":"hello","name":"Ana","local_players":[0,1]}"#, peer.control.last)
        peer.events.onControlMessage(#"{"type":"ping","id":7}"#)
        XCTAssertEqual(#"{"type":"pong","id":7}"#, peer.control.last)
        peer.events.onControlMessage(#"{"type":"chat","name":"Bo","text":"hola","ts":1}"#)
        peer.events.onState(.connected)
        XCTAssertEqual(1, room.ui.chat.count)
        XCTAssertEqual(.streaming, room.ui.phase)

        // A name with symbols is cleaned like the device does; guests ask the host for a pause.
        room.setIdentity(name: "  Ana ✨ #1 ", localPlayers: [0, 1])
        XCTAssertEqual(#"{"type":"hello","name":"Ana 1","local_players":[0,1]}"#, peer.control.last)
        room.requestPause()
        XCTAssertEqual(#"{"type":"pause_request"}"#, peer.control.last)
        room.cancelPauseRequest()
        XCTAssertEqual(#"{"type":"pause_request","cancel":true}"#, peer.control.last)

        // Input goes out at once and repeats while held.
        room.setPad(player: 0, pad: Pad(buttons: Button.b1))
        XCTAssertEqual(1, peer.input.count)
        clock.advance(by: 250)
        XCTAssertEqual(3, peer.input.count)
        room.setPad(player: 0, pad: .empty)
        clock.advance(by: 500)
        XCTAssertEqual(6, peer.input.count) // the release plus two repeats

        // The connection drops: a new one joins again and sends the token, never the used PIN.
        first.listener.onClosed()
        XCTAssertTrue(peer.closed)
        XCTAssertEqual(.joining, room.ui.phase)
        clock.advance(by: 2000)
        let second = sockets.last
        XCTAssertTrue(second !== first)
        second.receive(#"{"type":"hello","peer_id":"me2"}"#)
        second.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}"#)
        second.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
        XCTAssertEqual(#"{"kind":"pin","token":"\#(token)"}"#, payload(second.sent.last!))

        // A refused token is forgotten and the person is asked.
        second.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"wrong","left":4}}"#)
        XCTAssertEqual("", RoomPasses(store).get("room-1"))
        XCTAssertEqual(PinView(needed: true, busy: false, last: nil), room.ui.pin)
        XCTAssertEqual(["pin"], kinds(second))

        room.submitPin("111222")
        XCTAssertEqual(#"{"kind":"pin","pin":"111222"}"#, payload(second.sent.last!))
        second.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"used"}}"#)
        XCTAssertEqual("used", room.ui.pin.last?.reason)

        // The host leaves: the room has ended.
        second.receive(#"{"type":"peer_left","session_id":"s","from":"dev"}"#)
        XCTAssertEqual(.ended, room.ui.phase)
        room.close()
    }

    func testJoinErrors() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let signal = SignalClient(url: "wss://signal.example.org/ws", factory: sockets, scheduler: clock)
        let room = RoomClient(signal: signal, target: .code("123456789"), typedPin: "000000", passes: RoomPasses(MemoryStore()), peers: FakePeers(), scheduler: clock)
        room.start()
        sockets.last.receive(#"{"type":"hello","peer_id":"me"}"#)
        XCTAssertEqual([#"{"type":"join","app":"go-link","code":"123456789"}"#], sockets.last.sentText)
        sockets.last.receive(#"{"type":"error","error":"invalid or expired invite"}"#)
        XCTAssertEqual(.notFound, room.ui.phase)
        room.close()
    }

    func testRateLimitAndSilence() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let signal = SignalClient(url: "wss://signal.example.org/ws", factory: sockets, scheduler: clock)
        let room = RoomClient(signal: signal, target: .code("123456789"), typedPin: "000000", passes: RoomPasses(MemoryStore()), peers: FakePeers(), scheduler: clock)
        room.start()
        sockets.last.receive(#"{"type":"hello","peer_id":"me"}"#)
        sockets.last.receive(#"{"type":"error","error":"rate limit exceeded"}"#)
        XCTAssertEqual(.rateLimited, room.ui.phase)
        room.reconnect()
        sockets.last.receive(#"{"type":"hello","peer_id":"me2"}"#)
        clock.advance(by: 10_000) // no reply at all
        XCTAssertEqual(.unreachable, room.ui.phase)
        room.close()
    }

    func testControlMessagesWaitForTheChannel() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let peers = FakePeers()
        let signal = SignalClient(url: "wss://a.example/ws", factory: sockets, scheduler: clock)
        let room = RoomClient(signal: signal, target: .link(invite), typedPin: "123456", passes: RoomPasses(MemoryStore()), peers: peers, scheduler: clock)
        room.start()
        let c = sockets.last
        c.receive(#"{"type":"hello","peer_id":"me"}"#)
        c.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"r"}"#)
        c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"offer","sdp":"o"}}"#)
        room.sendChat("  hi  ")
        room.swapSeat(from: 1, to: 2)
        let peer = peers.created[0]
        XCTAssertTrue(peer.control.isEmpty)
        peer.events.onControlOpen()
        XCTAssertEqual([
            #"{"type":"hello","name":"","local_players":[0]}"#,
            #"{"type":"chat","text":"hi"}"#,
            #"{"type":"swap_seat","from":1,"to":2}"#,
        ], peer.control)
        peer.events.onControlMessage(#"{"type":"room_state","max_players":4,"seats":[{"name":"Me","you":true}],"you":{"ports":[1]}}"#)
        peer.events.onControlMessage(#"{"type":"stream_stats","fps":60,"aspect":1.25}"#)
        peer.events.onControlMessage(#"{"type":"stream_stats","fps":59}"#)
        XCTAssertEqual(.player([1]), room.ui.room?.me)
        XCTAssertEqual(StreamStatsView(fps: 59, aspect: 1.25), room.ui.stats)
        // The 2x picture's game size is kept until a new "video" arrives.
        peer.events.onControlMessage(#"{"type":"stream_stats","fps":60,"width":768,"height":448,"video":{"scale":2,"width":384,"height":224,"quality":"high"}}"#)
        peer.events.onControlMessage(#"{"type":"stream_stats","fps":60}"#)
        XCTAssertEqual(StreamVideo(scale: 2, width: 384, height: 224, quality: .high), room.ui.stats.video)
        XCTAssertEqual(PictureLayout.Size(w: 384, h: 224), room.ui.stats.video?.native)
        room.close()
    }

    private let key = String(repeating: "k", count: 43)

    private func groupRoom(_ sockets: FakeSockets, _ clock: ManualScheduler, _ store: MemoryStore, _ peers: FakePeers) -> RoomClient {
        let signal = SignalClient(url: "wss://signal.example.org/ws", factory: sockets, scheduler: clock, random: { 0.5 })
        let room = RoomClient(signal: signal, target: .link(invite), typedPin: "", key: key, keyName: "  Ana  ", passes: RoomPasses(store), peers: peers, scheduler: clock)
        room.start()
        sockets.last.receive(#"{"type":"hello","peer_id":"me1"}"#)
        sockets.last.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}"#)
        return room
    }

    func testGroupInvitationKnocksWaitsAndComesIn() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let store = MemoryStore()
        let peers = FakePeers()
        let room = groupRoom(sockets, clock, store, peers)
        let first = sockets.last

        // No PIN: the key and the name go out.
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
        XCTAssertEqual(#"{"key":"\#(key)","kind":"pin","name":"Ana"}"#, payload(first.sent.last!))
        XCTAssertEqual(.pin, room.ui.phase)
        XCTAssertTrue(room.ui.pin.busy)

        // The host decides.
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"waiting"}}"#)
        XCTAssertEqual(.waiting, room.ui.knock)
        XCTAssertEqual(.pin, room.ui.phase)

        // Let in: the return token is saved and the knock is over.
        first.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":true,"token":"\#(token)"}}"#)
        XCTAssertEqual(.none, room.ui.knock)
        XCTAssertEqual(.connecting, room.ui.phase)
        XCTAssertEqual(token, RoomPasses(store).get("room-1"))

        // A dropped connection gets back in with the token, not the key.
        first.listener.onClosed()
        clock.advance(by: 2000)
        let second = sockets.last
        second.receive(#"{"type":"hello","peer_id":"me2"}"#)
        second.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}"#)
        second.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
        XCTAssertEqual(#"{"kind":"pin","token":"\#(token)"}"#, payload(second.sent.last!))
        // An old token: the key is sent again.
        second.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"wrong","left":4}}"#)
        XCTAssertEqual(#"{"key":"\#(key)","kind":"pin","name":"Ana"}"#, payload(second.sent.last!))
        room.close()
    }

    func testGroupInvitationRefusedStops() {
        for reason in ["declined", "full", "expired", "busy", "wrong", "blocked", "locked", ""] {
            let clock = ManualScheduler()
            let sockets = FakeSockets()
            let room = groupRoom(sockets, clock, MemoryStore(), FakePeers())
            let c = sockets.last
            c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
            c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"waiting"}}"#)
            c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"\#(reason)"}}"#)
            XCTAssertEqual(.ended(reason.isEmpty ? "wrong" : reason), room.ui.knock, reason)
            // Nothing else is tried: no new join after the connection closes.
            XCTAssertTrue(c.closed)
            clock.advance(by: 60_000)
            XCTAssertEqual(1, sockets.conns.count, reason)
            XCTAssertEqual(1, kinds(c).filter { $0 == "pin" }.count, reason)
            room.close()
        }
    }

    func testRemovedByTheHostEndsTheVisit() {
        let clock = ManualScheduler()
        let sockets = FakeSockets()
        let store = MemoryStore()
        let peers = FakePeers()
        let signal = SignalClient(url: "wss://signal.example.org/ws", factory: sockets, scheduler: clock, random: { 0.5 })
        let room = RoomClient(signal: signal, target: .link(invite), typedPin: "482913", passes: RoomPasses(store), peers: peers, scheduler: clock)
        room.start()
        let c = sockets.last
        c.receive(#"{"type":"hello","peer_id":"me1"}"#)
        c.receive(#"{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}"#)
        c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}"#)
        c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":true,"token":"\#(token)"}}"#)
        c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"offer","sdp":"o1"}}"#)
        let peer = peers.created[0]
        peer.events.onState(.connected)
        XCTAssertEqual(.streaming, room.ui.phase)

        // Raised hands and refusals travel on control.
        peer.events.onControlOpen()
        room.raiseHand(true)
        XCTAssertEqual(#"{"type":"hand","on":true}"#, peer.control.last)
        room.raiseHand(false)
        XCTAssertEqual(#"{"type":"hand","on":false}"#, peer.control.last)
        peer.events.onControlMessage(#"{"type":"error","code":"hand_too_soon","error":"wait"}"#)
        XCTAssertEqual(Refusal(code: "hand_too_soon", seq: 1), room.ui.refused)
        peer.events.onControlMessage(#"{"type":"error","code":"hand_too_soon","error":"wait"}"#)
        XCTAssertEqual(Refusal(code: "hand_too_soon", seq: 2), room.ui.refused)

        // The host takes this guest out: the stream ends and nothing is tried again.
        c.receive(#"{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"removed"}}"#)
        XCTAssertEqual(.removed, room.ui.phase)
        XCTAssertTrue(peer.closed)
        XCTAssertTrue(c.closed)
        // The token is kept (it would only answer "removed" again).
        XCTAssertEqual(token, RoomPasses(store).get("room-1"))
        room.reconnect()
        clock.advance(by: 60_000)
        XCTAssertEqual(1, sockets.conns.count)
        XCTAssertEqual(.removed, room.ui.phase)
        room.close()
    }
}
