// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player

import android.content.Context
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import org.golink.player.audio.AudioRouter
import org.golink.player.core.AudioChoice
import org.golink.player.core.InviteTarget
import org.golink.player.core.LiveStatsMeter
import org.golink.player.core.LiveStatsView
import org.golink.player.core.MAX_LOCAL_PLAYERS
import org.golink.player.core.Pad
import org.golink.player.core.RoomClient
import org.golink.player.core.RoomPasses
import org.golink.player.core.RtcPeerFactory
import org.golink.player.core.SignalClient
import org.golink.player.core.VoiceSlot
import org.golink.player.core.VoiceSlots
import org.golink.player.input.ConnectedController
import org.golink.player.input.Controllers
import org.golink.player.input.GamepadInput
import org.golink.player.net.OkHttpSockets
import org.golink.player.rtc.AndroidRtcPeer
import org.golink.player.rtc.MediaListener
import org.golink.player.rtc.WebRtcEngine
import org.webrtc.AudioTrack
import org.webrtc.VideoTrack

/** What the room screen shows about sound and voice. */
data class SoundState(
    val gameMuted: Boolean = false,
    val micOn: Boolean = false,
    /**
     * People whose voice this player silenced, by name: a voice track
     * carries whoever speaks now, so a seat or a track is not the person.
     */
    val silenced: Set<String> = emptySet(),
    /** Voice slots (1-4) whose track arrived. */
    val voices: Set<Int> = emptySet(),
    /** The game's volume, 0 to 1 (1 = as sent). */
    val gameVolume: Double = 1.0,
    /** The other players' voices, 0 to 1. */
    val voiceVolume: Double = 1.0,
)

/**
 * One visit to a room: the signaling connection, the room logic from
 * :core, the WebRTC peer, the audio route and the controllers. It lives in
 * the ViewModel, so turning the phone keeps the game running.
 */
class RoomSession(
    context: Context,
    private val prefs: Prefs,
    val target: InviteTarget,
    pin: String,
    private val scope: CoroutineScope,
    /** A group invitation's knock: the name the host sees ("" for a PIN). */
    knockName: String = "",
) {
    private val appContext = context.applicationContext
    private val engine = WebRtcEngine.get(context)
    val eglContext get() = engine.eglBase.eglBaseContext
    private val router = AudioRouter(context, prefs) { engine.setPreferredInput(it) }
    val audioOutput = router.output

    /** The sound settings' rows and choices. */
    val audioChoices = router.choices

    /** A chosen sound device disconnected (the choice went back to Automatic). */
    val audioLost = router.lost

    private var peer: AndroidRtcPeer? = null

    /** The current WebRTC connection, for the debug build's diagnostics. */
    val rtcPeer: AndroidRtcPeer? get() = peer
    private var stopProbe: () -> Unit = {}
    private var gameAudio: AudioTrack? = null
    private val voiceTracks = HashMap<Int, AudioTrack>()

    private val _video = MutableStateFlow<VideoTrack?>(null)
    val video: StateFlow<VideoTrack?> = _video.asStateFlow()

    private val _sound = MutableStateFlow(SoundState(gameVolume = prefs.gameVolume, voiceVolume = prefs.voiceVolume))
    val sound: StateFlow<SoundState> = _sound.asStateFlow()

    private val _controllers = MutableStateFlow<List<GamepadInput.Controller>>(emptyList())
    val controllers: StateFlow<List<GamepadInput.Controller>> = _controllers.asStateFlow()

    /** Physical controllers the system lists (used or not yet), refreshed on connect and disconnect. */
    private val _connected = MutableStateFlow(Controllers.connected(appContext))
    val connected: StateFlow<List<ConnectedController>> = _connected.asStateFlow()

    /** The buttons held on all real controllers, for the see-through pad. */
    private val _controllerButtons = MutableStateFlow(0)
    val controllerButtons: StateFlow<Int> = _controllerButtons.asStateFlow()

    /** Whether the person already confirmed the name for this visit (the alias step). */
    // A knock already gave the name: no alias step.
    private val _aliasDone = MutableStateFlow(knockName.isNotEmpty())
    val aliasDone: StateFlow<Boolean> = _aliasDone.asStateFlow()

    /** The stats overlay's values, while it is on; null while off or before the first reading. */
    private val _liveStats = MutableStateFlow<LiveStatsView?>(null)
    val liveStats: StateFlow<LiveStatsView?> = _liveStats.asStateFlow()
    private var statsJob: Job? = null

    val gamepads = GamepadInput { scope.launch(Dispatchers.Main.immediate) { pushPads() } }
    private var touchButtons = 0
    private val lastPads = MutableList(MAX_LOCAL_PLAYERS) { Pad.EMPTY }

    private val media = object : MediaListener {
        override fun onVideo(track: VideoTrack) {
            scope.launch(Dispatchers.Main) { _video.value = track }
        }

        override fun onGameAudio(track: AudioTrack) {
            scope.launch(Dispatchers.Main) {
                gameAudio = track
                applyVolumes()
            }
        }

        override fun onVoice(slot: Int, track: AudioTrack) {
            scope.launch(Dispatchers.Main) {
                voiceTracks[slot] = track
                _sound.update { it.copy(voices = it.voices + slot) }
                applyVolumes()
            }
        }

        override fun onClosed() {
            _video.value = null
            gameAudio = null
            voiceTracks.clear()
            _sound.update { it.copy(voices = emptySet()) }
        }
    }

    private val signal = SignalClient(prefs.signal.url, OkHttpSockets(), scope)

    val client = RoomClient(
        signal = signal,
        target = target,
        typedPin = pin,
        passes = RoomPasses(prefs),
        peers = RtcPeerFactory { servers, events ->
            // Runs on the main thread (RoomClient's scope).
            AndroidRtcPeer(engine, servers, events, media).also { p ->
                peer = p
                applyMic()
            }
        },
        scope = scope,
        knockName = knockName,
    )

    // Who each voice slot carries and whether this phone may talk, from room_state.
    private var voiceSlots: List<VoiceSlot?> = List(4) { null }
    private var talker = false
    private var roomJob: Job? = null

    fun start() {
        router.start()
        client.setIdentity(prefs.playerName, listOf(0))
        client.start()
        roomJob = scope.launch {
            client.ui.collect { ui ->
                val room = ui.room ?: return@collect
                // The players and the queue talk; a spectator's microphone stays off.
                val canTalk = room.canTalk && room.voice
                if (room.voiceSlots != voiceSlots) {
                    voiceSlots = room.voiceSlots
                    applyVolumes()
                }
                if (canTalk != talker) {
                    talker = canTalk
                    applyMic()
                }
            }
        }
        // Debug builds log the room and the RTP counters for the end-to-end
        // test (src/debug); release builds have an empty probe (src/release).
        stopProbe = E2eProbe.attach(this, scope)
    }

    fun close() {
        roomJob?.cancel()
        showStats(false)
        stopProbe()
        client.close()
        peer = null
        router.stop()
        gamepads.clear()
    }

    /** Re-picks the audio device (a permission changed, a headset came). */
    fun reroute() = router.route()

    fun chooseAudioOutput(choice: AudioChoice) = router.chooseOutput(choice)

    fun chooseAudioInput(choice: AudioChoice) = router.chooseInput(choice)

    /** A short chime on the current output. */
    fun testSound() = router.testTone()

    fun setName(name: String) = client.setIdentity(name, localPlayers())

    /** The alias step: the name was confirmed (and saved by the caller). */
    fun confirmAlias(name: String) {
        _aliasDone.value = true
        setName(name)
    }

    /** A controller was connected, changed or removed. */
    fun refreshControllers() {
        _connected.value = Controllers.connected(appContext)
    }

    /** Reads libwebrtc's statistics once a second while the overlay is on. */
    fun showStats(on: Boolean) {
        statsJob?.cancel()
        statsJob = null
        _liveStats.value = null
        if (!on) return
        val meter = LiveStatsMeter()
        statsJob = scope.launch {
            var last: AndroidRtcPeer? = null
            while (true) {
                val p = peer
                if (p !== last) {
                    meter.reset() // a new connection starts its counters over
                    last = p
                }
                p?.sample { sample -> scope.launch(Dispatchers.Main) { if (statsJob != null && peer === p) _liveStats.value = meter.update(sample) } }
                delay(1_000)
            }
        }
    }

    /** Buttons held on the on-screen gamepad: they belong to local player 0. */
    fun setTouchButtons(buttons: Int) {
        if (buttons == touchButtons) return
        touchButtons = buttons
        pushPads()
    }

    private fun localPlayers(): List<Int> = (listOf(0) + gamepads.controllers.map { it.player }).distinct().sorted()

    private fun pushPads() {
        val pads = gamepads.pads().toMutableList()
        _controllerButtons.value = pads.fold(0) { acc, p -> acc or p.buttons }
        pads[0] = pads[0].copy(buttons = pads[0].buttons or touchButtons)
        pads.forEachIndexed { player, pad ->
            if (pad != lastPads[player]) {
                lastPads[player] = pad
                client.setPad(player, pad)
            }
        }
        val list = gamepads.controllers
        if (list != _controllers.value) {
            _controllers.value = list
            client.setIdentity(prefs.playerName, localPlayers())
        }
    }

    fun setGameMuted(muted: Boolean) {
        _sound.update { it.copy(gameMuted = muted) }
        applyVolumes()
    }

    fun setGameVolume(v: Double) {
        prefs.gameVolume = v
        _sound.update { it.copy(gameVolume = prefs.gameVolume) }
        applyVolumes()
    }

    fun setVoiceVolume(v: Double) {
        prefs.voiceVolume = v
        _sound.update { it.copy(voiceVolume = prefs.voiceVolume) }
        applyVolumes()
    }

    /** Silences (or not) one person's voice, only for this phone, wherever they sit or wait. */
    fun toggleSilence(name: String) {
        if (name.isEmpty()) return
        _sound.update { s -> s.copy(silenced = if (name in s.silenced) s.silenced - name else s.silenced + name) }
        applyVolumes()
    }

    /** Turns the microphone on or off. Call on() only with RECORD_AUDIO granted. */
    fun setMic(on: Boolean) {
        _sound.update { it.copy(micOn = on) }
        applyMic()
    }

    private fun applyMic() {
        val p = peer ?: return
        p.setMicTrack(if (_sound.value.micOn && talker) engine.microphone() else null)
    }

    private fun applyVolumes() {
        val s = _sound.value
        // libwebrtc's remote volume: 0 to 10, 1 = as received.
        gameAudio?.setVolume(if (s.gameMuted) 0.0 else s.gameVolume)
        val muted = VoiceSlots.silencedSlots(voiceSlots, s.silenced)
        for ((slot, track) in voiceTracks) track.setVolume(if (slot in muted) 0.0 else s.voiceVolume)
    }
}
