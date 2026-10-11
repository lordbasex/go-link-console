// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class RtcSignalTest {
    private val token = "a".repeat(43)

    @Test
    fun pinGateMessages() {
        assertEquals(DeviceSignal.PinRequired, RtcSignals.parse(parseObject("""{"kind":"pin_required"}""")))
        assertEquals(
            DeviceSignal.PinResult(true, token, "", 0, 0),
            RtcSignals.parse(parseObject("""{"kind":"pin_result","ok":true,"token":"$token"}""")),
        )
        assertEquals(
            DeviceSignal.PinResult(false, "", "locked", 0, 600),
            RtcSignals.parse(parseObject("""{"kind":"pin_result","ok":false,"reason":"locked","retry_after":600,"token":"short"}""")),
        )
        assertEquals("""{"kind":"pin","pin":"123456"}""", RtcSignals.pin("123456").toString())
        assertEquals("""{"kind":"pin","token":"$token"}""", RtcSignals.token(token).toString())
    }

    @Test
    fun groupKeyAndItsAnswers() {
        val key = "K".repeat(43)
        assertEquals("""{"kind":"pin","key":"$key","name":"Ana"}""", RtcSignals.key(key, "Ana").toString())
        // The name the host sees is cut to 24 characters.
        assertEquals("""{"kind":"pin","key":"$key","name":"${"n".repeat(24)}"}""", RtcSignals.key(key, "n".repeat(30)).toString())
        assertEquals(
            DeviceSignal.PinResult(false, "", "waiting", 0, 0),
            RtcSignals.parse(parseObject("""{"kind":"pin_result","ok":false,"reason":"waiting"}""")),
        )
        assertEquals(
            DeviceSignal.PinResult(false, "", "removed", 0, 0),
            RtcSignals.parse(parseObject("""{"kind":"pin_result","ok":false,"reason":"removed"}""")),
        )
    }

    @Test
    fun negotiation() {
        assertEquals(DeviceSignal.Offer("v=0"), RtcSignals.parse(parseObject("""{"kind":"offer","sdp":"v=0"}""")))
        assertEquals(
            DeviceSignal.Candidate(IceCandidateInit("candidate:1 1 udp 1 192.0.2.1 5000 typ host", "0", 0)),
            RtcSignals.parse(parseObject("""{"kind":"candidate","candidate":{"candidate":"candidate:1 1 udp 1 192.0.2.1 5000 typ host","sdpMid":"0","sdpMLineIndex":0}}""")),
        )
        assertNull(RtcSignals.parse(parseObject("""{"kind":"offer"}""")))
        assertNull(RtcSignals.parse(parseObject("""{"kind":"other"}""")))
        assertEquals(
            """{"kind":"candidate","candidate":{"candidate":"c","sdpMid":"1","sdpMLineIndex":1}}""",
            RtcSignals.candidate(IceCandidateInit("c", "1", 1)).toString(),
        )
    }

    @Test
    fun microphoneLine() {
        val sdp = listOf(
            "v=0", "o=- 1 1 IN IP4 0.0.0.0", "s=-",
            "m=video 9 UDP/TLS/RTP/SAVPF 96", "a=mid:0", "a=sendonly",
            "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:1", "a=sendonly",
            "m=audio 9 UDP/TLS/RTP/SAVPF 111", "a=mid:6", "a=recvonly",
        ).joinToString("\r\n")
        assertEquals("6", RtcSignals.micMid(sdp))
        assertNull(RtcSignals.micMid("v=0\r\nm=audio 9 X 111\r\na=mid:1\r\na=sendonly"))
        assertEquals(2, RtcSignals.voiceSlot("voice-p2"))
        assertNull(RtcSignals.voiceSlot("go-link"))
    }
}
