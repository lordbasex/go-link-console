// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

final class RtcSignalTests: XCTestCase {
    private let token = String(repeating: "a", count: 43)

    func testPinGateMessages() {
        XCTAssertEqual(.pinRequired, RtcSignals.parse(JSONText.parse(#"{"kind":"pin_required"}"#)))
        XCTAssertEqual(
            .pinResult(PinResult(ok: true, token: token, reason: "", left: 0, retryAfter: 0)),
            RtcSignals.parse(JSONText.parse(#"{"kind":"pin_result","ok":true,"token":"\#(token)"}"#))
        )
        XCTAssertEqual(
            .pinResult(PinResult(ok: false, token: "", reason: "locked", left: 0, retryAfter: 600)),
            RtcSignals.parse(JSONText.parse(#"{"kind":"pin_result","ok":false,"reason":"locked","retry_after":600,"token":"short"}"#))
        )
        XCTAssertEqual(#"{"kind":"pin","pin":"123456"}"#, RtcSignals.pin("123456").text)
        XCTAssertEqual(#"{"kind":"pin","token":"\#(token)"}"#, RtcSignals.token(token).text)
    }

    func testGroupInvitationKeyMessage() {
        XCTAssertEqual(#"{"kind":"pin","key":"\#(token)","name":"Ana"}"#, RtcSignals.key(token, name: "Ana").text)
        // The name the host sees is 24 characters at most.
        let long = String(repeating: "n", count: 30)
        XCTAssertEqual(#"{"kind":"pin","key":"\#(token)","name":"\#(String(repeating: "n", count: 24))"}"#, RtcSignals.key(token, name: long).text)
    }

    func testRemovedAndWaitingResults() {
        XCTAssertEqual(
            .pinResult(PinResult(ok: false, token: "", reason: "removed", left: 0, retryAfter: 0)),
            RtcSignals.parse(JSONText.parse(#"{"kind":"pin_result","ok":false,"reason":"removed"}"#))
        )
        XCTAssertEqual(
            .pinResult(PinResult(ok: false, token: "", reason: "waiting", left: 0, retryAfter: 0)),
            RtcSignals.parse(JSONText.parse(#"{"kind":"pin_result","ok":false,"reason":"waiting"}"#))
        )
    }

    func testNegotiation() {
        XCTAssertEqual(.offer("v=0"), RtcSignals.parse(JSONText.parse(#"{"kind":"offer","sdp":"v=0"}"#)))
        XCTAssertEqual(
            .candidate(IceCandidateInit(candidate: "candidate:1 1 udp 1 192.0.2.1 5000 typ host", sdpMid: "0", sdpMLineIndex: 0)),
            RtcSignals.parse(JSONText.parse(#"{"kind":"candidate","candidate":{"candidate":"candidate:1 1 udp 1 192.0.2.1 5000 typ host","sdpMid":"0","sdpMLineIndex":0}}"#))
        )
        XCTAssertNil(RtcSignals.parse(JSONText.parse(#"{"kind":"offer"}"#)))
        XCTAssertNil(RtcSignals.parse(JSONText.parse(#"{"kind":"other"}"#)))
        XCTAssertEqual(
            #"{"kind":"candidate","candidate":{"candidate":"c","sdpMid":"1","sdpMLineIndex":1}}"#,
            RtcSignals.candidate(IceCandidateInit(candidate: "c", sdpMid: "1", sdpMLineIndex: 1)).text
        )
    }

    func testMicrophoneLine() {
        let sdp = [
            "v=0", "o=- 1 1 IN IP4 0.0.0.0", "s=-",
            "m=video 9 UDP/TLS/RTP/SAVPF 96", "a=mid:0", "a=sendonly",
            "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:1", "a=sendonly",
            "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:6", "a=recvonly",
        ].joined(separator: "\r\n")
        XCTAssertEqual("6", RtcSignals.micMid(sdp))
        XCTAssertNil(RtcSignals.micMid("v=0\r\nm=audio 9 X 111\r\na=mid:1\r\na=sendonly"))
        XCTAssertEqual(2, RtcSignals.voicePort("voice-p2"))
        XCTAssertNil(RtcSignals.voicePort("go-link"))
    }
}
