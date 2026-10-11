// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.rtc

import kotlinx.coroutines.suspendCancellableCoroutine
import org.golink.player.core.IceCandidateInit
import org.golink.player.core.IceServer
import org.golink.player.core.PeerState
import org.golink.player.core.RtcPeer
import org.golink.player.core.RtcPeerEvents
import org.golink.player.core.RtcSample
import org.golink.player.core.RtcSignals
import org.webrtc.AudioTrack
import org.webrtc.DataChannel
import org.webrtc.IceCandidate
import org.webrtc.MediaConstraints
import org.webrtc.MediaStream
import org.webrtc.PeerConnection
import org.webrtc.RTCStatsReport
import org.webrtc.RtpReceiver
import org.webrtc.RtpSender
import org.webrtc.RtpTransceiver
import org.webrtc.SdpObserver
import org.webrtc.SessionDescription
import org.webrtc.VideoTrack
import java.nio.ByteBuffer
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** Media the device sends, handed to the room screen. Called on WebRTC threads. */
interface MediaListener {
    fun onVideo(track: VideoTrack)

    fun onGameAudio(track: AudioTrack)

    /**
     * A voice slot (1-4): the forwarded voice of whoever speaks now
     * (room_state.voice_slots says who), not a seat.
     */
    fun onVoice(slot: Int, track: AudioTrack)

    /** The connection is closing: its tracks go away. Called on the closing thread. */
    fun onClosed()
}

/**
 * The app's side of the WebRTC link with the device, the Android
 * counterpart of the web's HostStream (frontend/packages/shared/src/stream.ts):
 * it answers the device's offer, receives the game's video and sound and
 * the voices, and speaks the "control" and "input" channels.
 */
class AndroidRtcPeer(
    private val engine: WebRtcEngine,
    iceServers: List<IceServer>,
    private val events: RtcPeerEvents,
    private val media: MediaListener,
) : RtcPeer {
    @Volatile private var control: DataChannel? = null

    @Volatile private var input: DataChannel? = null

    @Volatile private var micSender: RtpSender? = null

    @Volatile private var micTrack: AudioTrack? = null

    @Volatile private var closed = false

    /** What each received track carries, by track id: "video", "game" or "voice-pN". */
    private val trackLabels = ConcurrentHashMap<String, String>()

    private val pc: PeerConnection

    init {
        // STUN/TURN from signalhub's hello, never hardcoded.
        val servers = iceServers.map { s ->
            PeerConnection.IceServer.builder(s.urls)
                .setUsername(s.username ?: "")
                .setPassword(s.credential ?: "")
                .createIceServer()
        }
        val config = PeerConnection.RTCConfiguration(servers).apply {
            sdpSemantics = PeerConnection.SdpSemantics.UNIFIED_PLAN
            continualGatheringPolicy = PeerConnection.ContinualGatheringPolicy.GATHER_CONTINUALLY
        }
        pc = requireNotNull(engine.factory.createPeerConnection(config, Observer())) { "peer connection" }
    }

    override suspend fun answer(offerSdp: String): String {
        pc.awaitSet(SessionDescription(SessionDescription.Type.OFFER, offerSdp))
        prepareMicrophone(offerSdp)
        val answer = pc.awaitCreateAnswer()
        pc.awaitSet(answer, local = true)
        return answer.description
    }

    /** The device offers the microphone line as recvonly: answer it sendonly. */
    private fun prepareMicrophone(offer: String) {
        val mid = RtcSignals.micMid(offer) ?: return
        val t: RtpTransceiver = pc.transceivers.firstOrNull { it.mid == mid } ?: return
        t.direction = RtpTransceiver.RtpTransceiverDirection.SEND_ONLY
        micSender = t.sender
        micTrack?.let { t.sender.setTrack(it, false) }
    }

    /**
     * Sets the microphone (null stops sending). It swaps the track on the
     * line the device reserved for it, so nothing is renegotiated.
     */
    fun setMicTrack(track: AudioTrack?) {
        micTrack = track
        if (!closed) micSender?.setTrack(track, false)
    }

    /** What a received track carries ("video", "game", "voice-pN"), for diagnostics. */
    fun trackLabel(trackId: String): String? = trackLabels[trackId]

    /** libwebrtc's statistics (RTP counters, audio levels), delivered on a WebRTC thread. */
    fun stats(done: (RTCStatsReport) -> Unit) {
        if (!closed) pc.getStats { done(it) }
    }

    /**
     * One reading for the room's stats overlay (the video, the game sound
     * and the candidate pair in use), delivered on a WebRTC thread.
     */
    fun sample(done: (RtcSample) -> Unit) = stats { report -> done(toSample(report)) }

    private fun toSample(report: RTCStatsReport): RtcSample {
        val all = report.statsMap
        fun num(v: Any?): Long? = (v as? Number)?.toLong()
        var sample = RtcSample(atMs = report.timestampUs / 1000.0)
        var received = 0L
        var lost = 0L
        var counted = false
        for (s in all.values) {
            if (s.type != "inbound-rtp") continue
            val m = s.members
            val label = (m["trackIdentifier"] as? String)?.let { trackLabels[it] }
            val kind = m["kind"] as? String
            val codec = all[m["codecId"] as? String]?.members
            when {
                kind == "video" -> sample = sample.copy(
                    framesDecoded = num(m["framesDecoded"]),
                    frameWidth = num(m["frameWidth"])?.toInt(),
                    frameHeight = num(m["frameHeight"])?.toInt(),
                    videoMime = codec?.get("mimeType") as? String,
                )
                kind == "audio" && (label == "game" || label == null && sample.audioMime == null) -> sample = sample.copy(
                    audioMime = codec?.get("mimeType") as? String,
                    audioClockRate = num(codec?.get("clockRate"))?.toInt(),
                )
                else -> continue // the players' voices are not the game's stream
            }
            num(m["packetsReceived"])?.let { received += it; counted = true }
            num(m["packetsLost"])?.let { lost += it }
        }
        if (counted) sample = sample.copy(packetsReceived = received, packetsLost = lost)
        // The pair in use: the transport's selected pair, or the nominated one that succeeded.
        val selected = all.values.firstOrNull { it.type == "transport" }?.members?.get("selectedCandidatePairId") as? String
        val pair = all[selected] ?: all.values.firstOrNull {
            it.type == "candidate-pair" && it.members["nominated"] == true && it.members["state"] == "succeeded"
        }
        if (pair != null) {
            val local = all[pair.members["localCandidateId"] as? String]?.members
            val remote = all[pair.members["remoteCandidateId"] as? String]?.members
            sample = sample.copy(
                rttSeconds = (pair.members["currentRoundTripTime"] as? Number)?.toDouble(),
                localCandidateType = local?.get("candidateType") as? String,
                remoteCandidateType = remote?.get("candidateType") as? String,
            )
        }
        return sample
    }

    override suspend fun addCandidate(candidate: IceCandidateInit) {
        if (!closed) pc.addIceCandidate(IceCandidate(candidate.sdpMid ?: "", candidate.sdpMLineIndex, candidate.candidate))
    }

    override fun sendControl(text: String): Boolean {
        val ch = control ?: return false
        if (closed || ch.state() != DataChannel.State.OPEN) return false
        return ch.send(DataChannel.Buffer(ByteBuffer.wrap(text.toByteArray(Charsets.UTF_8)), false))
    }

    override fun sendInput(packet: ByteArray): Boolean {
        val ch = input ?: return false
        if (closed || ch.state() != DataChannel.State.OPEN) return false
        return ch.send(DataChannel.Buffer(ByteBuffer.wrap(packet), true))
    }

    override fun close() {
        if (closed) return
        closed = true
        media.onClosed()
        val c = control
        val i = input
        control = null
        input = null
        micSender = null
        // Disposing blocks on libwebrtc's threads: never on the main thread.
        disposer.execute {
            c?.unregisterObserver()
            c?.dispose()
            i?.dispose()
            pc.dispose()
        }
    }

    private inner class Observer : PeerConnection.Observer {
        override fun onIceCandidate(c: IceCandidate) {
            if (!closed) events.onLocalCandidate(IceCandidateInit(c.sdp, c.sdpMid, c.sdpMLineIndex))
        }

        override fun onConnectionChange(newState: PeerConnection.PeerConnectionState) {
            if (closed) return
            when (newState) {
                PeerConnection.PeerConnectionState.CONNECTED -> events.onState(PeerState.CONNECTED)
                PeerConnection.PeerConnectionState.FAILED -> events.onState(PeerState.FAILED)
                else -> Unit
            }
        }

        override fun onDataChannel(ch: DataChannel) {
            when (ch.label()) {
                "input" -> input = ch
                "control" -> {
                    control = ch
                    ch.registerObserver(object : DataChannel.Observer {
                        override fun onBufferedAmountChange(previousAmount: Long) = Unit

                        override fun onStateChange() {
                            if (ch.state() == DataChannel.State.OPEN && !closed) events.onControlOpen()
                        }

                        override fun onMessage(buffer: DataChannel.Buffer) {
                            if (buffer.binary || closed) return
                            val data = buffer.data
                            if (data.remaining() > 256 * 1024) return
                            val bytes = ByteArray(data.remaining())
                            data.get(bytes)
                            events.onControlMessage(String(bytes, Charsets.UTF_8))
                        }
                    })
                    if (ch.state() == DataChannel.State.OPEN) events.onControlOpen()
                }
                else -> ch.dispose() // "files" is only for the host's own browser
            }
        }

        override fun onAddTrack(receiver: RtpReceiver, streams: Array<out MediaStream>) {
            if (closed) return
            val id = streams.firstOrNull()?.id ?: ""
            when (val track = receiver.track()) {
                is VideoTrack -> {
                    trackLabels[track.id()] = "video"
                    media.onVideo(track)
                }
                is AudioTrack -> {
                    val slot = RtcSignals.voiceSlot(id)
                    trackLabels[track.id()] = if (slot != null) "voice-p$slot" else "game"
                    if (slot != null) media.onVoice(slot, track) else media.onGameAudio(track)
                }
            }
        }

        override fun onSignalingChange(state: PeerConnection.SignalingState) = Unit

        override fun onIceConnectionChange(state: PeerConnection.IceConnectionState) = Unit

        override fun onIceConnectionReceivingChange(receiving: Boolean) = Unit

        override fun onIceGatheringChange(state: PeerConnection.IceGatheringState) = Unit

        override fun onIceCandidatesRemoved(candidates: Array<out IceCandidate>) = Unit

        override fun onAddStream(stream: MediaStream) = Unit

        override fun onRemoveStream(stream: MediaStream) = Unit

        override fun onRenegotiationNeeded() = Unit
    }

    companion object {
        private val disposer = Executors.newSingleThreadExecutor()
    }
}

private suspend fun PeerConnection.awaitSet(desc: SessionDescription, local: Boolean = false) =
    suspendCancellableCoroutine { cont ->
        val observer = object : SdpObserver {
            override fun onCreateSuccess(p0: SessionDescription?) = Unit

            override fun onCreateFailure(p0: String?) = Unit

            override fun onSetSuccess() {
                if (cont.isActive) cont.resume(Unit)
            }

            override fun onSetFailure(error: String?) {
                if (cont.isActive) cont.resumeWithException(IllegalStateException(error ?: "set description"))
            }
        }
        if (local) setLocalDescription(observer, desc) else setRemoteDescription(observer, desc)
    }

private suspend fun PeerConnection.awaitCreateAnswer(): SessionDescription =
    suspendCancellableCoroutine { cont ->
        createAnswer(
            object : SdpObserver {
                override fun onCreateSuccess(desc: SessionDescription) {
                    if (cont.isActive) cont.resume(desc)
                }

                override fun onCreateFailure(error: String?) {
                    if (cont.isActive) cont.resumeWithException(IllegalStateException(error ?: "create answer"))
                }

                override fun onSetSuccess() = Unit

                override fun onSetFailure(p0: String?) = Unit
            },
            MediaConstraints(),
        )
    }
