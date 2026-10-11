// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Combine
import Foundation
import GoLinkCore
import UIKit
@preconcurrency import WebRTC

/** What the room screen shows about sound and voice. */
struct SoundState: Equatable {
    var micOn = false
    /**
     * People whose voice this player silenced, by name: a voice track is a
     * slot that carries whoever speaks now, so a seat or a track is not
     * the person.
     */
    var silenced = Set<String>()
    /** Voice slots (1-4) whose track arrived. */
    var voices = Set<Int>()
    /** 0 to 1 (0-100 %; 1 = as sent). */
    var gameVolume = 1.0
    var voiceVolume = 1.0
}

/**
 * One visit to a room: the signaling connection, the room logic from
 * GoLinkCore, the WebRTC peer, the audio route and the controllers. The
 * iOS counterpart of the Android app's RoomSession.
 */
@MainActor
final class RoomSession: ObservableObject {
    let target: InviteTarget
    let client: RoomClient
    let router: AudioRouter
    let gamepads: GamepadInput

    @Published private(set) var ui = RoomUi()
    @Published private(set) var video: RTCVideoTrack?
    @Published private(set) var sound = SoundState()
    @Published private(set) var controllers: [GamepadInput.Controller] = []
    /** Every connected controller (used or not): with one, the on-screen pad only shows it. */
    @Published private(set) var connectedControllers: [GamepadInput.Connected] = []
    /** The buttons held on the real controllers, to light the see-through pad. */
    @Published private(set) var controllerBits = 0
    /**
     * The name step: shown once per visit, when the PIN has been accepted
     * and before the room (never again after a reconnect in this visit).
     */
    @Published private(set) var askName = true
    /** The stats overlay's numbers, while it is on (nil until the first reading). */
    @Published private(set) var liveStats: LiveStatsView?
    /** Who speaks now (seats, and names in the queue), from the voice tracks' levels. */
    @Published private(set) var speaking = Speaking()
    /** "Do you want to join the voice?" (see GoLinkCore.VoiceAsk). */
    @Published private(set) var voiceAsk = false

    private let prefs: Prefs
    private let engine = WebRtcEngine.shared
    private let signal: SignalClient
    private(set) var peer: IOSRtcPeer?
    private var gameAudio: RTCAudioTrack?
    private var voiceTracks: [Int: RTCAudioTrack] = [:]
    private var touchButtons = 0
    private var lastPads = [Pad](repeating: .empty, count: maxLocalPlayers)
    private var uiToken: CancelHandle?
    private var bag = Set<AnyCancellable>()
    private var stopProbe: () -> Void = {}
    private let statsMeter = LiveStatsMeter()
    private var statsTimer: Timer?
    private var levelsTimer: Timer?
    private var voiceAskState = VoiceAsk()
    private var talker = false

    /**
     * pin is the typed PIN of a one-person invitation; a group invitation
     * sends its key with name instead (the key stays in memory only).
     */
    init(prefs: Prefs, invitation: Invitation, pin: String, name: String = "") {
        self.prefs = prefs
        target = invitation.target
        signal = SignalClient(url: prefs.signal.url, factory: URLSessionSockets(), scheduler: MainScheduler.shared)
        router = AudioRouter(prefs: prefs, device: WebRtcEngine.shared.audioDevice)
        let peers = PeerFactory()
        client = RoomClient(
            signal: signal, target: invitation.target, typedPin: pin, key: invitation.key, keyName: name,
            passes: RoomPasses(prefs), peers: peers, scheduler: MainScheduler.shared
        )
        // A group invitation already asked the name, for the host.
        askName = !invitation.isGroup
        var pushPads: () -> Void = {}
        gamepads = GamepadInput { pushPads() }
        pushPads = { [weak self] in self?.pushPads() }
        peers.session = self
        sound.gameVolume = prefs.gameVolume.clamped(0, 1)
        sound.voiceVolume = prefs.voiceVolume.clamped(0, 1)
    }

    /** Creates the WebRTC peer when the device offers (RoomClient asks for it). */
    private final class PeerFactory: RtcPeerFactory {
        weak var session: RoomSession?

        func create(iceServers: [IceServer], events: RtcPeerEvents) -> RtcPeer {
            let s = session!
            let p = IOSRtcPeer(engine: s.engine, iceServers: iceServers, events: events, media: s)
            s.peer = p
            s.applyMic()
            return p
        }
    }

    func start() {
        uiToken = client.onUi { [weak self] in self?.onUi($0) }
        gamepads.$controllers.sink { [weak self] list in
            guard let self else { return }
            self.controllers = list
            self.client.setIdentity(name: self.prefs.playerName, localPlayers: self.localPlayers(list))
        }.store(in: &bag)
        gamepads.$connected.sink { [weak self] list in self?.connectedControllers = list }.store(in: &bag)
        router.start()
        gamepads.start()
        client.setIdentity(name: prefs.playerName, localPlayers: [0])
        client.start()
        UIApplication.shared.isIdleTimerDisabled = true
        stopProbe = E2eProbe.attach(self)
    }

    func close() {
        setStatsOn(false)
        setLevelsOn(false)
        stopProbe()
        uiToken?.cancel()
        bag = []
        client.close()
        peer = nil
        engine.audioDevice.micEnabled = false
        router.stop()
        gamepads.stop()
        UIApplication.shared.isIdleTimerDisabled = false
    }

    func leave() { close() }

    private func onUi(_ next: RoomUi) {
        let slotsChanged = next.room?.voiceSlots != ui.room?.voiceSlots
        ui = next
        if slotsChanged { applyVolumes() }
        // The players and the queue talk: the microphone only sends while this app is one of them.
        let canTalk = next.room?.talker ?? false
        if canTalk != talker {
            talker = canTalk
            applyMic()
        }
        updateVoiceAsk()
    }

    private func updateVoiceAsk() {
        let shown = voiceAskState.update(talker: talker, roomVoice: ui.room?.voice != false, micOn: sound.micOn)
        if shown != voiceAsk { voiceAsk = shown }
    }

    /** The voice question's "Not now". */
    func dismissVoiceAsk() {
        voiceAskState.dismiss()
        voiceAsk = false
    }

    func setName(_ name: String) {
        prefs.playerName = name
        client.setIdentity(name: prefs.playerName, localPlayers: localPlayers(gamepads.controllers))
    }

    /** The name step's "Enter the room": saves the name and tells the device. */
    func confirmName(_ name: String) {
        setName(name)
        askName = false
    }

    // MARK: Stats

    /** Reads libwebrtc's statistics once a second while the overlay is on. */
    func setStatsOn(_ on: Bool) {
        statsTimer?.invalidate()
        statsTimer = nil
        statsMeter.reset()
        liveStats = nil
        guard on else { return }
        let t = Timer(timeInterval: 1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.readStats() }
        }
        RunLoop.main.add(t, forMode: .common)
        statsTimer = t
        readStats()
    }

    /** Reads the voice tracks' levels a few times a second while there are voices, to show who speaks. */
    private func setLevelsOn(_ on: Bool) {
        if on == (levelsTimer != nil) { return }
        levelsTimer?.invalidate()
        levelsTimer = nil
        guard on else {
            if speaking != Speaking() { speaking = Speaking() }
            return
        }
        let t = Timer(timeInterval: 0.3, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.readLevels() }
        }
        RunLoop.main.add(t, forMode: .common)
        levelsTimer = t
    }

    private func readLevels() {
        guard let peer else { return }
        peer.stats { [weak self] report in
            guard let self, self.levelsTimer != nil else { return }
            let levels = LiveStatsReader.voiceLevels(report, labels: peer.trackLabels)
            let now = VoiceSlots.speaking(slots: self.ui.room?.voiceSlots ?? [], levels: levels, silenced: self.sound.silenced)
            if now != self.speaking { self.speaking = now }
        }
    }

    private func readStats() {
        guard let peer else { return }
        peer.stats { [weak self] report in
            guard let self, self.statsTimer != nil else { return }
            self.liveStats = self.statsMeter.update(LiveStatsReader.sample(report, labels: peer.trackLabels))
        }
    }

    /** Buttons held on the on-screen gamepad: they belong to local player 0. */
    func setTouchButtons(_ buttons: Int) {
        guard buttons != touchButtons else { return }
        touchButtons = buttons
        pushPads()
    }

    private func localPlayers(_ list: [GamepadInput.Controller]) -> [Int] {
        Array(Set([0] + list.map(\.player))).sorted()
    }

    private func pushPads() {
        var pads = gamepads.pads()
        let fromControllers = pads.reduce(0) { $0 | $1.buttons }
        if fromControllers != controllerBits { controllerBits = fromControllers }
        pads[0].buttons |= touchButtons
        for (player, pad) in pads.enumerated() where pad != lastPads[player] {
            lastPads[player] = pad
            client.setPad(player: player, pad: pad)
        }
    }

    // MARK: Sound

    func setGameVolume(_ v: Double) {
        sound.gameVolume = v.clamped(0, 1)
        prefs.gameVolume = sound.gameVolume
        applyVolumes()
    }

    func setVoiceVolume(_ v: Double) {
        sound.voiceVolume = v.clamped(0, 1)
        prefs.voiceVolume = sound.voiceVolume
        applyVolumes()
    }

    /** The dock's switch: game sound off and back to the chosen volume. */
    @Published private(set) var gameMuted = false

    func setGameMuted(_ muted: Bool) {
        gameMuted = muted
        applyVolumes()
    }

    /** Silences (or not) one person's voice, by name, only for this device. */
    func toggleSilence(_ name: String) {
        guard !name.isEmpty else { return }
        if sound.silenced.contains(name) { sound.silenced.remove(name) } else { sound.silenced.insert(name) }
        applyVolumes()
    }

    /** Turns the microphone on or off. Call on() only with the microphone allowed. */
    func setMic(_ on: Bool) {
        sound.micOn = on
        applyMic()
        updateVoiceAsk()
    }

    func testSound() { engine.audioDevice.chime() }

    fileprivate func applyMic() {
        // Only the players and the queue talk (the device drops a spectator's voice too).
        let on = sound.micOn && talker
        engine.audioDevice.micEnabled = on
        peer?.setMicTrack(on ? engine.microphone() : nil)
    }

    private func applyVolumes() {
        // libwebrtc's remote volume goes from 0 to 10 (1 = as received).
        gameAudio?.source.volume = gameMuted ? 0 : sound.gameVolume
        let muted = VoiceSlots.mutedSlots(slots: ui.room?.voiceSlots ?? [], silenced: sound.silenced)
        for (slot, track) in voiceTracks {
            track.source.volume = muted.contains(slot) ? 0 : sound.voiceVolume
        }
    }
}

extension RoomSession: MediaListener {
    func onVideo(_ track: RTCVideoTrack) { video = track }

    func onGameAudio(_ track: RTCAudioTrack) {
        gameAudio = track
        applyVolumes()
    }

    func onVoice(slot: Int, track: RTCAudioTrack) {
        voiceTracks[slot] = track
        sound.voices.insert(slot)
        applyVolumes()
        setLevelsOn(true)
    }

    func onMediaClosed() {
        video = nil
        gameAudio = nil
        voiceTracks = [:]
        sound.voices = []
        setLevelsOn(false)
    }
}
