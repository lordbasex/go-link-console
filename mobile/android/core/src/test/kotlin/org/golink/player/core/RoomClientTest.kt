// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** A signalhub stand-in: records what the client sends and lets tests answer. */
class FakeSockets : SignalSocketFactory {
    inner class Conn(val url: String, val listener: SignalSocketListener) : SignalSocket {
        val sent = mutableListOf<JsonObject>()
        var closed = false

        override fun send(text: String): Boolean {
            sent += parseObject(text)!!
            return !closed
        }

        override fun close() {
            closed = true
        }

        fun receive(text: String) = listener.onMessage(text)
    }

    val conns = mutableListOf<Conn>()
    val last get() = conns.last()

    override fun open(url: String, listener: SignalSocketListener): SignalSocket = Conn(url, listener).also { conns += it }
}

class FakePeer : RtcPeer {
    val control = mutableListOf<String>()
    val input = mutableListOf<ByteArray>()
    val candidates = mutableListOf<IceCandidateInit>()
    var offers = mutableListOf<String>()
    var closed = false
    lateinit var events: RtcPeerEvents

    override suspend fun answer(offerSdp: String): String {
        offers += offerSdp
        return "answer-for-$offerSdp"
    }

    override suspend fun addCandidate(candidate: IceCandidateInit) {
        candidates += candidate
    }

    override fun sendControl(text: String): Boolean {
        control += text
        return true
    }

    override fun sendInput(packet: ByteArray): Boolean {
        input += packet
        return true
    }

    override fun close() {
        closed = true
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class RoomClientTest {
    private val invite = "AbCdEfGhIjKlMnOpQr_-12"
    private val token = "t".repeat(43)

    private fun payloadOf(msg: JsonObject): JsonObject = msg["payload"] as JsonObject

    private fun kinds(conn: FakeSockets.Conn) =
        conn.sent.filter { it["type"] == JsonPrimitive("signal") }.map { payloadOf(it)["kind"]!!.toString().trim('"') }

    @Test
    fun joinsSendsThePinAnswersAndComesBackWithTheToken() = runTest {
        val sockets = FakeSockets()
        val store = MemoryStore()
        val peers = mutableListOf<FakePeer>()
        val signal = SignalClient("wss://signal.example.org/ws", sockets, backgroundScope, random = kotlin.random.Random(1))
        val room = RoomClient(
            signal,
            InviteTarget.Link(invite),
            "482913",
            RoomPasses(store),
            { servers, events ->
                assertEquals(listOf(IceServer(listOf("stun:signal.example.org:3478"))), servers)
                FakePeer().also { it.events = events; peers += it }
            },
            backgroundScope,
        )
        room.start()
        runCurrent()
        val first = sockets.last
        assertEquals("wss://signal.example.org/ws?v=1", first.url)
        first.receive("""{"type":"hello","peer_id":"me1","ice_servers":[{"urls":["stun:signal.example.org:3478"]}]}""")
        runCurrent()
        assertEquals("""{"type":"join","app":"go-link-console","invite":"$invite"}""", first.sent.single().toString())

        // The device asks for the PIN right after the join: the typed PIN goes out.
        first.receive("""{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}""")
        first.receive("""{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}""")
        runCurrent()
        assertEquals(RoomPhase.PIN, room.ui.value.phase)
        assertEquals("""{"kind":"pin","pin":"482913"}""", payloadOf(first.sent.last()).toString())
        assertEquals("dev", (first.sent.last()["to"] as JsonPrimitive).content)

        first.receive("""{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":true,"token":"$token"}}""")
        runCurrent()
        assertEquals(RoomPhase.CONNECTING, room.ui.value.phase)
        assertEquals(token, RoomPasses(store).get("room-1"))

        // Offer, early candidate, answer; a signal from someone else is ignored.
        first.receive("""{"type":"signal","from":"dev","payload":{"kind":"candidate","candidate":{"candidate":"c1","sdpMid":"0","sdpMLineIndex":0}}}""")
        first.receive("""{"type":"signal","from":"dev","payload":{"kind":"offer","sdp":"o1"}}""")
        first.receive("""{"type":"signal","from":"intruder","payload":{"kind":"offer","sdp":"evil"}}""")
        runCurrent()
        val peer = peers.single()
        assertEquals(listOf("o1"), peer.offers)
        assertEquals(listOf(IceCandidateInit("c1", "0", 0)), peer.candidates)
        assertEquals("""{"kind":"answer","sdp":"answer-for-o1"}""", payloadOf(first.sent.last()).toString())

        // The control channel opens: hello with the name and local players; pings are answered.
        room.setIdentity("Ana", listOf(1, 0, 1))
        peer.events.onControlOpen()
        runCurrent()
        assertEquals("""{"type":"hello","name":"Ana","local_players":[0,1]}""", peer.control.last())
        peer.events.onControlMessage("""{"type":"ping","id":7}""")
        peer.events.onControlMessage("""{"type":"chat","name":"Bo","text":"hola","ts":1}""")
        peer.events.onState(PeerState.CONNECTED)
        runCurrent()
        assertEquals("""{"type":"pong","id":7}""", peer.control.last())
        assertEquals(1, room.ui.value.chat.size)
        assertEquals(RoomPhase.STREAMING, room.ui.value.phase)

        // A 2x picture: the game's size stays until a new "video" arrives.
        peer.events.onControlMessage("""{"type":"stream_stats","fps":60,"aspect":1.25,"video":{"scale":2,"width":384,"height":224,"quality":"high"}}""")
        peer.events.onControlMessage("""{"type":"stream_stats","fps":59}""")
        runCurrent()
        assertEquals(StreamStatsView(59.0, 1.25, StreamVideo(2, 384, 224, VideoQuality.HIGH)), room.ui.value.stats)
        assertEquals(PixelSize(384, 224), room.ui.value.stats.video?.native)

        // A name with symbols is cleaned like the device does; guests ask the host for a pause.
        room.setIdentity("  Ana ✨ #1 ", listOf(0, 1))
        runCurrent()
        assertEquals("""{"type":"hello","name":"Ana 1","local_players":[0,1]}""", peer.control.last())
        room.requestPause()
        runCurrent()
        assertEquals("""{"type":"pause_request"}""", peer.control.last())
        room.cancelPauseRequest()
        runCurrent()
        assertEquals("""{"type":"pause_request","cancel":true}""", peer.control.last())

        // Input goes out at once and repeats while held.
        room.setPad(0, Pad(Button.B1))
        runCurrent()
        assertEquals(1, peer.input.size)
        advanceTimeBy(250)
        assertEquals(3, peer.input.size)
        room.setPad(0, Pad.EMPTY)
        advanceTimeBy(500)
        val afterRelease = peer.input.size
        assertEquals(6, afterRelease) // the release plus two repeats

        // The connection drops: a new one joins again and sends the token, never the used PIN.
        first.listener.onClosed()
        runCurrent()
        assertTrue(peer.closed)
        assertEquals(RoomPhase.JOINING, room.ui.value.phase)
        advanceTimeBy(2_000)
        val second = sockets.last
        assertTrue(second !== first)
        second.receive("""{"type":"hello","peer_id":"me2"}""")
        runCurrent()
        second.receive("""{"type":"joined","session_id":"s","remote":"dev","room_id":"room-1"}""")
        second.receive("""{"type":"signal","from":"dev","payload":{"kind":"pin_required"}}""")
        runCurrent()
        assertEquals("""{"kind":"pin","token":"$token"}""", payloadOf(second.sent.last()).toString())

        // A refused token is forgotten and the person is asked.
        second.receive("""{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"wrong","left":4}}""")
        runCurrent()
        assertEquals("", RoomPasses(store).get("room-1"))
        assertEquals(PinView(needed = true, busy = false, last = null), room.ui.value.pin)
        assertEquals(listOf("pin"), kinds(second))

        room.submitPin("111222")
        runCurrent()
        assertEquals("""{"kind":"pin","pin":"111222"}""", payloadOf(second.sent.last()).toString())
        second.receive("""{"type":"signal","from":"dev","payload":{"kind":"pin_result","ok":false,"reason":"used"}}""")
        runCurrent()
        assertEquals("used", room.ui.value.pin.last?.reason)

        // The host leaves: the room has ended.
        second.receive("""{"type":"peer_left","session_id":"s","from":"dev"}""")
        runCurrent()
        assertEquals(RoomPhase.ENDED, room.ui.value.phase)
        room.close()
    }

    @Test
    fun joinErrors() = runTest {
        val sockets = FakeSockets()
        val signal = SignalClient("wss://signal.example.org/ws", sockets, backgroundScope)
        val room = RoomClient(signal, InviteTarget.Code("123456789"), "000000", RoomPasses(MemoryStore()), { _, _ -> FakePeer() }, backgroundScope)
        room.start()
        runCurrent()
        sockets.last.receive("""{"type":"hello","peer_id":"me"}""")
        runCurrent()
        assertEquals("""{"type":"join","app":"go-link-console","code":"123456789"}""", sockets.last.sent.single().toString())
        sockets.last.receive("""{"type":"error","error":"invalid or expired invite"}""")
        runCurrent()
        assertEquals(RoomPhase.NOT_FOUND, room.ui.value.phase)
        room.close()
    }
}
