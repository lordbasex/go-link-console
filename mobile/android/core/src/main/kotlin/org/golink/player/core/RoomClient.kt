// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** The WebRTC side of a room, implemented by the platform (libwebrtc on Android). */
interface RtcPeer {
    /** Applies the device's offer and returns the answer's SDP (the microphone line answered sendonly). */
    suspend fun answer(offerSdp: String): String

    suspend fun addCandidate(candidate: IceCandidateInit)

    /** Sends JSON text on the reliable "control" DataChannel. */
    fun sendControl(text: String): Boolean

    /** Sends one binary packet on the unreliable "input" DataChannel. */
    fun sendInput(packet: ByteArray): Boolean

    fun close()
}

enum class PeerState { CONNECTING, CONNECTED, FAILED, CLOSED }

/** Peer events. They may arrive on any thread: RoomClient moves them onto its scope. */
interface RtcPeerEvents {
    fun onLocalCandidate(candidate: IceCandidateInit)

    fun onState(state: PeerState)

    fun onControlOpen()

    fun onControlMessage(text: String)
}

fun interface RtcPeerFactory {
    fun create(iceServers: List<IceServer>, events: RtcPeerEvents): RtcPeer
}

enum class RoomPhase {
    /** Waiting for signaling or the join reply (also while getting back in after a drop). */
    JOINING,

    /** The device asks for the invitation's PIN (or is checking one). */
    PIN,

    /** A group invitation's knock: the host decides whether this person comes in. */
    WAITING,

    /** The group invitation did not let this person in (RoomUi.knockEnd says why). Final. */
    REFUSED,

    /** The host took this guest out of the room: only a new invitation brings them back. Final. */
    REMOVED,

    /** Admitted: the WebRTC connection is being set up. */
    CONNECTING,

    /** Media flows. */
    STREAMING,

    /** The host left or closed the room. */
    ENDED,

    /** The invitation or code does not exist (or expired). */
    NOT_FOUND,

    /** Too many attempts from this network. */
    RATE_LIMITED,

    /** The signaling server does not answer. */
    UNREACHABLE,
}

/** The PIN the room asks for. last is the most recent refusal, to explain it. */
data class PinView(val needed: Boolean = false, val busy: Boolean = false, val last: DeviceSignal.PinResult? = null)

data class RoomUi(
    val phase: RoomPhase = RoomPhase.JOINING,
    /** signalhub room_id, known once joined. */
    val roomId: String = "",
    val reconnecting: Boolean = false,
    val pin: PinView = PinView(),
    val room: RoomStateView? = null,
    val chat: List<ChatLine> = emptyList(),
    val typing: List<TypingView> = emptyList(),
    val stats: StreamStatsView = StreamStatsView(null, null),
    val controlOpen: Boolean = false,
    /**
     * Why a group invitation's knock ended (phase REFUSED): declined,
     * full, expired, busy, wrong, blocked or locked.
     */
    val knockEnd: String = "",
    /** The last request the device refused, by its error code. */
    val refused: Refusal? = null,
)

/** A refused request: code (hand_too_soon, queue_ask_host...), n counts them so a repeat shows again. */
data class Refusal(val code: String, val n: Int)

/**
 * One room visit: joins through an invitation, passes the PIN gate,
 * answers the device's WebRTC offer, and speaks the "control" and "input"
 * channels. A port of the web's useJoinRoom + useHostStream + HostStream
 * (frontend/apps/web/src/signal, frontend/packages/shared/src/stream.ts),
 * without the media, which the platform's RtcPeer handles.
 *
 * PIN rule (the user's decision): a new join always sends the PIN the
 * person typed. The return token from pin_result is used only to get back
 * in automatically after the connection drops during this visit.
 *
 * A group invitation (a link with #k=<key>) sends its key and the
 * person's name instead of a PIN, then waits while the host decides
 * (pin_result "waiting"). The key stays in memory only.
 *
 * Every method must be called on scope's dispatcher.
 */
class RoomClient(
    private val signal: SignalClient,
    private val target: InviteTarget,
    typedPin: String,
    private val passes: RoomPasses,
    private val peers: RtcPeerFactory,
    private val scope: CoroutineScope,
    /** The name the host sees when knocking with a group invitation's key. */
    private val knockName: String = "",
) {
    private val _ui = MutableStateFlow(RoomUi())
    val ui: StateFlow<RoomUi> = _ui.asStateFlow()

    private var pinToSend: String? = typedPin.takeIf { Invites.isPin(it) }
    private var sentToken = false
    private var admittedOnce = false

    // A group invitation: its key is sent once per join, with the name.
    private val groupKey = (target as? InviteTarget.Link)?.key ?: ""
    private var knockSent = false
    private var knocking = false

    // Stopped for good (taken out by the host, or a knock that ended): no more tries.
    private var halted = false

    private var joinedOn = "" // peer_id of the signaling connection that joined
    private var hostPeerId = ""
    private var joining = false
    private var backlog = mutableListOf<Envelope>()
    private var peer: RtcPeer? = null
    private var remoteSet = false
    private val pendingCandidates = mutableListOf<IceCandidateInit>()
    private val pendingControl = mutableListOf<String>()
    private var controlOpen = false
    private var closed = false
    private var peerGeneration = 0

    private var name = ""
    private var localPlayers: List<Int> = listOf(0)

    private val input = InputSender(scope) { packet -> peer?.sendInput(packet) ?: false }

    private var offListener: (() -> Unit)? = null
    private var stateJob: Job? = null

    fun start() {
        offListener = signal.addListener(::onEnvelope)
        stateJob = scope.launch {
            signal.state.collect { s ->
                if (closed || halted) return@collect
                if (s == ConnectionState.OPEN) {
                    if (joinedOn != signal.peerId && !joining) join()
                } else if (joinedOn.isNotEmpty()) {
                    // Link lost: drop the media and get back in when it returns.
                    joinedOn = ""
                    dropPeer()
                    _ui.update { it.copy(phase = RoomPhase.JOINING, reconnecting = true, controlOpen = false) }
                }
            }
        }
        signal.connect()
    }

    /** Leaves the room: the signaling connection is reset, as on the web. */
    fun close() {
        if (closed) return
        closed = true
        input.releaseAll()
        offListener?.invoke()
        stateJob?.cancel()
        dropPeer()
        signal.close()
    }

    /** Gets back in (new signaling connection, same visit): used after a failure. */
    fun reconnect() {
        if (closed || halted) return
        joinedOn = ""
        dropPeer()
        _ui.update { it.copy(phase = RoomPhase.JOINING, reconnecting = true, controlOpen = false) }
        signal.reset()
    }

    /** Stops for good with a final phase: no more joins, the media goes away. */
    private fun halt(update: (RoomUi) -> RoomUi) {
        halted = true
        input.releaseAll()
        dropPeer()
        hostPeerId = ""
        offListener?.invoke()
        offListener = null
        stateJob?.cancel()
        signal.close()
        _ui.update { update(it).copy(controlOpen = false) }
    }

    private fun join() {
        joining = true
        knockSent = false
        knocking = false
        val peerAtJoin = signal.peerId
        backlog = mutableListOf()
        hostPeerId = ""
        _ui.update { it.copy(phase = RoomPhase.JOINING) }
        scope.launch {
            try {
                val env = signal.request(Envelopes.join(target), setOf("joined"))
                if (closed) return@launch
                joinedOn = peerAtJoin
                hostPeerId = env.remote
                val roomId = env.roomId
                _ui.update { it.copy(roomId = roomId) }
                val early = backlog.toList()
                backlog.clear()
                early.forEach(::onEnvelope)
            } catch (e: SignalException) {
                if (closed) return@launch
                if (e.fromServer) {
                    val phase = if (e.message == ServerError.RATE_LIMITED) RoomPhase.RATE_LIMITED else RoomPhase.NOT_FOUND
                    // A room that vanished while getting back in has ended.
                    val ended = _ui.value.reconnecting && phase == RoomPhase.NOT_FOUND
                    _ui.update { it.copy(phase = if (ended) RoomPhase.ENDED else phase) }
                } else if (e.message != "disconnected") {
                    _ui.update { it.copy(phase = RoomPhase.UNREACHABLE) }
                }
            } finally {
                joining = false
            }
        }
    }

    private fun onEnvelope(env: Envelope) {
        if (closed) return
        when (env.type) {
            "signal" -> {
                if (hostPeerId.isEmpty()) {
                    if (joining && backlog.size < 100) backlog.add(env)
                    return
                }
                if (env.from != hostPeerId) return
                val sig = RtcSignals.parse(env.payload) ?: return
                handleSignal(sig)
            }
            "peer_left" -> if (hostPeerId.isNotEmpty() && env.from == hostPeerId) {
                dropPeer()
                hostPeerId = ""
                _ui.update { it.copy(phase = RoomPhase.ENDED, controlOpen = false) }
            }
        }
    }

    private fun sendSignal(payload: JsonObject) {
        if (hostPeerId.isEmpty()) return
        signal.send(Envelopes.signal(hostPeerId, payload))
    }

    private fun handleSignal(sig: DeviceSignal) {
        when (sig) {
            DeviceSignal.PinRequired -> onPinRequired(null)
            is DeviceSignal.PinResult -> onPinResult(sig)
            is DeviceSignal.Offer -> scope.launch { applyOffer(sig.sdp) }
            is DeviceSignal.Candidate -> scope.launch { addRemoteCandidate(sig.candidate) }
        }
    }

    private fun onPinRequired(last: DeviceSignal.PinResult?) {
        val roomId = _ui.value.roomId
        // Getting back in after a drop: the token this app got on the way in.
        val token = if (admittedOnce && !sentToken && roomId.isNotEmpty()) passes.get(roomId) else ""
        if (token.isNotEmpty()) {
            sentToken = true
            _ui.update { it.copy(phase = RoomPhase.PIN, pin = PinView(needed = true, busy = true)) }
            sendSignal(RtcSignals.token(token))
            return
        }
        if (groupKey.isNotEmpty() && !knockSent) {
            // The group invitation: the key and a name the host recognises.
            knockSent = true
            knocking = true
            _ui.update { it.copy(phase = RoomPhase.WAITING, pin = PinView(needed = true, busy = true)) }
            sendSignal(RtcSignals.key(groupKey, knockName))
            return
        }
        val pin = pinToSend
        if (pin != null) {
            pinToSend = null // an invitation's PIN works once
            _ui.update { it.copy(phase = RoomPhase.PIN, pin = PinView(needed = true, busy = true)) }
            sendSignal(RtcSignals.pin(pin))
            return
        }
        _ui.update { it.copy(phase = RoomPhase.PIN, pin = PinView(needed = true, busy = false, last = last)) }
    }

    private fun onPinResult(r: DeviceSignal.PinResult) {
        val roomId = _ui.value.roomId
        if (r.reason == "removed") {
            // Taken out by the host: the token now always answers "removed"
            // (never a wrong PIN), so it is kept and nothing else is tried.
            halt { it.copy(phase = RoomPhase.REMOVED, pin = PinView()) }
            return
        }
        if (r.ok) {
            admittedOnce = true
            sentToken = false
            knocking = false
            if (r.token.isNotEmpty() && roomId.isNotEmpty()) passes.save(roomId, r.token)
            _ui.update { it.copy(phase = RoomPhase.CONNECTING, pin = PinView(), reconnecting = false) }
            return
        }
        if (knocking) {
            if (r.reason == "waiting") {
                _ui.update { it.copy(phase = RoomPhase.WAITING, pin = PinView(needed = true, busy = true, last = r)) }
                return
            }
            // The group invitation did not let this person in: say why.
            knocking = false
            halt { it.copy(phase = RoomPhase.REFUSED, knockEnd = r.reason.ifEmpty { "wrong" }, pin = PinView()) }
            return
        }
        if (sentToken) {
            // An old token: forget it and ask the person.
            sentToken = false
            admittedOnce = false
            if (roomId.isNotEmpty()) passes.forget(roomId)
            onPinRequired(null)
            return
        }
        onPinRequired(r)
    }

    /** A PIN the person typed on the room's PIN prompt. */
    fun submitPin(pin: String) {
        if (!Invites.isPin(pin) || hostPeerId.isEmpty()) return
        _ui.update { it.copy(pin = it.pin.copy(busy = true)) }
        sendSignal(RtcSignals.pin(pin))
    }

    private fun ensurePeer(): RtcPeer {
        peer?.let { return it }
        val gen = ++peerGeneration
        val events = object : RtcPeerEvents {
            override fun onLocalCandidate(candidate: IceCandidateInit) {
                scope.launch { if (gen == peerGeneration) sendSignal(RtcSignals.candidate(candidate)) }
            }

            override fun onState(state: PeerState) {
                scope.launch { if (gen == peerGeneration) onPeerState(state) }
            }

            override fun onControlOpen() {
                scope.launch { if (gen == peerGeneration) onControlOpened() }
            }

            override fun onControlMessage(text: String) {
                scope.launch { if (gen == peerGeneration) onControl(text) }
            }
        }
        // STUN/TURN come from signalhub's hello; nothing is hardcoded.
        val p = peers.create(signal.iceServers, events)
        peer = p
        remoteSet = false
        return p
    }

    private suspend fun applyOffer(sdp: String) {
        val p = ensurePeer()
        val gen = peerGeneration
        try {
            val answer = p.answer(sdp)
            if (gen != peerGeneration) return
            remoteSet = true
            for (c in pendingCandidates.toList()) p.addCandidate(c)
            pendingCandidates.clear()
            sendSignal(RtcSignals.answer(answer))
            if (_ui.value.phase != RoomPhase.STREAMING) _ui.update { it.copy(phase = RoomPhase.CONNECTING) }
        } catch (_: Exception) {
            if (gen == peerGeneration) onPeerState(PeerState.FAILED)
        }
    }

    private suspend fun addRemoteCandidate(c: IceCandidateInit) {
        val p = peer
        if (p != null && remoteSet) {
            runCatching { p.addCandidate(c) }
        } else if (pendingCandidates.size < 50) {
            // Before the answer a device sends a handful, never hundreds.
            pendingCandidates.add(c)
        }
    }

    private fun onPeerState(state: PeerState) {
        when (state) {
            PeerState.CONNECTED -> _ui.update { it.copy(phase = RoomPhase.STREAMING, reconnecting = false) }
            PeerState.FAILED -> if (!closed && _ui.value.phase != RoomPhase.ENDED) reconnect()
            else -> Unit
        }
    }

    private fun dropPeer() {
        peerGeneration++
        peer?.close()
        peer = null
        remoteSet = false
        pendingCandidates.clear()
        controlOpen = false
        input.reset()
    }

    private fun onControlOpened() {
        controlOpen = true
        sendHello()
        for (t in pendingControl.toList()) peer?.sendControl(t)
        pendingControl.clear()
        _ui.update { it.copy(controlOpen = true) }
    }

    private fun onControl(text: String) {
        val m = parseObject(text) ?: return
        if (m["type"].str(40) == "ping") {
            // The device measures the peer-to-peer round trip with pings.
            peer?.sendControl(buildJsonObject { put("type", "pong"); m["id"]?.let { put("id", it) } }.toString())
            return
        }
        RoomMessages.parseRoomState(m)?.let { s -> _ui.update { it.copy(room = s) }; return }
        RoomMessages.parseChat(m)?.let { line -> _ui.update { it.copy(chat = (it.chat + line).takeLast(MAX_CHAT_LINES)) }; return }
        RoomMessages.parseTyping(m)?.let { who -> _ui.update { it.copy(typing = who) }; return }
        RoomMessages.parseRefusal(m)?.let { code -> _ui.update { it.copy(refused = Refusal(code, (it.refused?.n ?: 0) + 1)) }; return }
        RoomMessages.parseStreamStats(m)?.let { st ->
            // The last valid "video" stays (like the website) until a new one arrives.
            _ui.update { it.copy(stats = StreamStatsView(st.fps ?: it.stats.fps, st.aspect ?: it.stats.aspect, st.video ?: it.stats.video)) }
        }
    }

    /**
     * Sends a JSON message on "control". Messages sent before the channel
     * opens are kept (up to 20) and flushed on open.
     */
    fun sendControl(message: JsonObject) {
        val text = message.toString()
        if (!controlOpen || peer?.sendControl(text) != true) {
            if (pendingControl.size < 20) pendingControl.add(text)
        }
    }

    private fun sendHello() {
        val hello = buildJsonObject {
            put("type", "hello")
            put("name", name)
            put("local_players", JsonArray(localPlayers.map { JsonPrimitive(it) }))
        }
        if (controlOpen) peer?.sendControl(hello.toString())
    }

    /** Your name and which local players (touch + gamepads) want seats. */
    fun setIdentity(name: String, localPlayers: List<Int>) {
        val players = localPlayers.filter { it in 0 until MAX_LOCAL_PLAYERS }.distinct().sorted().ifEmpty { listOf(0) }
        // The device's rules (PlayerName): an unusable name is sent empty and the device picks one.
        val clean = PlayerName.sanitize(name)
        if (clean == this.name && players == this.localPlayers) return
        this.name = clean
        this.localPlayers = players
        sendHello()
    }

    fun setPad(player: Int, pad: Pad) = input.setPad(player, pad)

    fun sendChat(text: String) {
        val t = text.trim().take(300)
        if (t.isNotEmpty()) sendControl(buildJsonObject { put("type", "chat"); put("text", t) })
    }

    fun sendTyping(on: Boolean) = sendControl(buildJsonObject { put("type", "typing"); put("on", on) })

    fun spectate() = sendControl(buildJsonObject { put("type", "spectate") })

    /**
     * Back to the queue from watching. Someone the host moved to watching
     * (you.held) gets the error queue_ask_host instead: they raise a hand.
     */
    fun joinQueue() = sendControl(buildJsonObject { put("type", "queue") })

    /** A spectator raises (or lowers) a hand to ask the host to play. */
    fun raiseHand(on: Boolean) = sendControl(buildJsonObject { put("type", "hand"); put("on", on) })

    /**
     * Only the host pauses and resumes a game: a guest asks for a pause
     * (the answer comes as room_state.paused, or a pause_declined chat
     * notice), or withdraws its request with cancel.
     */
    fun requestPause() = sendControl(buildJsonObject { put("type", "pause_request") })

    fun cancelPauseRequest() = sendControl(buildJsonObject { put("type", "pause_request"); put("cancel", true) })

    fun swapSeat(from: Int, to: Int) = sendControl(buildJsonObject { put("type", "swap_seat"); put("from", from); put("to", to) })

    fun answerSwap(from: Int, to: Int, accept: Boolean) =
        sendControl(buildJsonObject { put("type", "swap_answer"); put("from", from); put("to", to); put("accept", accept) })

    companion object {
        const val MAX_CHAT_LINES = 200
    }
}

/**
 * Sends each local player's controller state on "input": on every change,
 * and every 100 ms while anything is held, because the channel drops lost
 * packets instead of retransmitting. Two extra releases follow a release.
 */
class InputSender(private val scope: CoroutineScope, private val send: (ByteArray) -> Boolean) {
    private class Player(var seq: Int = 0, var pad: Pad = Pad.EMPTY, var repeat: Job? = null)

    private val players = HashMap<Int, Player>()

    fun setPad(player: Int, pad: Pad) {
        if (player !in 0 until MAX_LOCAL_PLAYERS) return
        val p = players.getOrPut(player) { Player() }
        val changed = pad != p.pad
        p.pad = pad
        if (changed) sendNow(player)
        if (!pad.isIdle && p.repeat == null) {
            p.repeat = scope.launch {
                while (true) {
                    delay(100)
                    sendNow(player)
                }
            }
        } else if (pad.isIdle && p.repeat != null) {
            p.repeat?.cancel()
            p.repeat = null
            scope.launch {
                delay(50)
                sendNow(player)
                delay(100)
                sendNow(player)
            }
        }
    }

    fun releaseAll() {
        for (k in players.keys.toList()) setPad(k, Pad.EMPTY)
    }

    /** Forgets sequences and timers (a new connection starts over). */
    fun reset() {
        players.values.forEach { it.repeat?.cancel() }
        players.clear()
    }

    private fun sendNow(player: Int) {
        val p = players[player] ?: return
        p.seq = (p.seq + 1) and 0xffff
        send(InputPacket.encode(p.seq, player, p.pad))
    }
}
