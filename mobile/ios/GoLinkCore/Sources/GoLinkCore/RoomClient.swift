// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation

/** The WebRTC side of a room, implemented by the platform (libwebrtc on iOS). */
@MainActor
public protocol RtcPeer: AnyObject {
    /** Applies the device's offer and returns the answer's SDP (the microphone line answered sendonly). */
    func answer(offerSdp: String, done: @escaping (Result<String, Error>) -> Void)

    func addCandidate(_ candidate: IceCandidateInit)

    /** Sends JSON text on the reliable "control" DataChannel. */
    @discardableResult
    func sendControl(_ text: String) -> Bool

    /** Sends one binary packet on the unreliable "input" DataChannel. */
    @discardableResult
    func sendInput(_ packet: [UInt8]) -> Bool

    func close()
}

public enum PeerState: Sendable { case connecting, connected, failed, closed }

/** Peer events. The platform delivers them on the main actor. */
@MainActor
public protocol RtcPeerEvents: AnyObject {
    func onLocalCandidate(_ candidate: IceCandidateInit)
    func onState(_ state: PeerState)
    func onControlOpen()
    func onControlMessage(_ text: String)
}

@MainActor
public protocol RtcPeerFactory: AnyObject {
    func create(iceServers: [IceServer], events: RtcPeerEvents) -> RtcPeer
}

public enum RoomPhase: String, Sendable {
    /** Waiting for signaling or the join reply (also while getting back in after a drop). */
    case joining
    /** The device asks for the invitation's PIN (or is checking one). */
    case pin
    /** Admitted: the WebRTC connection is being set up. */
    case connecting
    /** Media flows. */
    case streaming
    /** The host left or closed the room. */
    case ended
    /** The invitation or code does not exist (or expired). */
    case notFound
    /** Too many attempts from this network. */
    case rateLimited
    /** The signaling server does not answer. */
    case unreachable
    /** The host took this guest out of the room: only a new invitation brings it back. */
    case removed
}

/**
 * A group invitation's way in: the key went out with a name, and the host
 * may have to let the person in. ended carries why it closed: declined,
 * full, expired, busy, wrong, blocked or locked.
 */
public enum KnockState: Equatable, Sendable {
    case none
    case waiting
    case ended(String)
}

/** A request the device refused (control "error"): code, and a number that changes with every refusal. */
public struct Refusal: Equatable, Sendable {
    public let code: String
    public let seq: Int

    public init(code: String, seq: Int) {
        self.code = code
        self.seq = seq
    }
}

/** The PIN the room asks for. last is the most recent refusal, to explain it. */
public struct PinView: Equatable, Sendable {
    public var needed = false
    public var busy = false
    public var last: PinResult?

    public init(needed: Bool = false, busy: Bool = false, last: PinResult? = nil) {
        self.needed = needed
        self.busy = busy
        self.last = last
    }
}

public struct RoomUi: Equatable, Sendable {
    public var phase: RoomPhase = .joining
    /** signalhub room_id, known once joined. */
    public var roomId = ""
    public var reconnecting = false
    public var pin = PinView()
    public var room: RoomStateView?
    public var chat: [ChatLine] = []
    public var typing: [TypingView] = []
    public var stats = StreamStatsView(fps: nil, aspect: nil)
    public var controlOpen = false
    /** The group invitation's state (none for a PIN). */
    public var knock = KnockState.none
    /** The last request the device refused, for a short notice. */
    public var refused: Refusal?

    public init() {}
}

/**
 * One room visit: joins through an invitation, passes the PIN gate,
 * answers the device's WebRTC offer, and speaks the "control" and "input"
 * channels. A port of the Android app's RoomClient.kt (itself a port of the
 * web's useJoinRoom + useHostStream + HostStream), without the media, which
 * the platform's RtcPeer handles.
 *
 * PIN rule (the user's decision): a new join always sends the PIN the
 * person typed. The return token from pin_result is used only to get back
 * in automatically after the connection drops during this visit.
 *
 * A group invitation sends its key and the person's name instead of a PIN,
 * on every ask (the host may answer "waiting" first). The key lives only in
 * this object's memory.
 */
@MainActor
public final class RoomClient {
    public static let maxChatLines = 200

    private let signal: SignalClient
    private let target: InviteTarget
    private let passes: RoomPasses
    private let peers: RtcPeerFactory
    private let scheduler: Scheduler

    public private(set) var ui = RoomUi() {
        didSet {
            if ui != oldValue {
                for (_, fn) in uiListeners { fn(ui) }
            }
        }
    }

    private var uiListeners: [(Int, (RoomUi) -> Void)] = []
    private var nextListener = 0

    private var pinToSend: String?
    private var sentToken = false
    private var admittedOnce = false
    private let groupKey: String
    private let keyName: String
    private var knocking = false
    /** The visit is over for good (taken out by the host, or the group invitation refused): no more joins. */
    private var halted = false
    private var refusals = 0

    private var joinedOn = "" // peer_id of the signaling connection that joined
    private var hostPeerId = ""
    private var joining = false
    private var backlog: [Envelope] = []
    private var peer: RtcPeer?
    private var remoteSet = false
    private var pendingCandidates: [IceCandidateInit] = []
    private var pendingControl: [String] = []
    private var controlOpen = false
    private var closed = false
    private var peerGeneration = 0

    private var name = ""
    private var localPlayers = [0]

    private lazy var input = InputSender(scheduler: scheduler) { [weak self] packet in
        self?.peer?.sendInput(packet) ?? false
    }

    private var envelopeToken: CancelHandle?
    private var stateToken: CancelHandle?

    /**
     * typedPin is the PIN of a one-person invitation; key and keyName, a
     * group invitation's key (43 base64url characters) and the name the
     * person gave for the host.
     */
    public init(signal: SignalClient, target: InviteTarget, typedPin: String, key: String = "", keyName: String = "", passes: RoomPasses, peers: RtcPeerFactory, scheduler: Scheduler) {
        self.signal = signal
        self.target = target
        self.passes = passes
        self.peers = peers
        self.scheduler = scheduler
        pinToSend = Invites.isPin(typedPin) ? typedPin : nil
        groupKey = RtcSignals.tokenOf(.string(key))
        self.keyName = keyName.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /** Called with every new ui value; the result stops listening. */
    @discardableResult
    public func onUi(_ fn: @escaping (RoomUi) -> Void) -> CancelHandle {
        nextListener += 1
        let id = nextListener
        uiListeners.append((id, fn))
        return UiToken { [weak self] in self?.uiListeners.removeAll { $0.0 == id } }
    }

    private final class UiToken: CancelHandle {
        let fn: () -> Void
        init(_ fn: @escaping () -> Void) { self.fn = fn }
        func cancel() { fn() }
    }

    public func start() {
        envelopeToken = signal.addListener { [weak self] env in self?.onEnvelope(env) }
        stateToken = signal.addStateListener { [weak self] s in self?.onSignalState(s) }
        signal.connect()
        onSignalState(signal.state)
    }

    private func onSignalState(_ s: ConnectionState) {
        if closed || halted { return }
        if s == .open {
            if joinedOn != signal.peerId && !joining { join() }
        } else if !joinedOn.isEmpty {
            // Link lost: drop the media and get back in when it returns.
            joinedOn = ""
            dropPeer()
            ui.phase = .joining
            ui.reconnecting = true
            ui.controlOpen = false
        }
    }

    /** Leaves the room: the signaling connection is closed, as on the web. */
    public func close() {
        if closed { return }
        closed = true
        input.releaseAll()
        envelopeToken?.cancel()
        stateToken?.cancel()
        dropPeer()
        signal.close()
    }

    /** Gets back in (new signaling connection, same visit): used after a failure. */
    public func reconnect() {
        if closed || halted { return }
        joinedOn = ""
        dropPeer()
        ui.phase = .joining
        ui.reconnecting = true
        ui.controlOpen = false
        signal.reset()
    }

    private func join() {
        joining = true
        let peerAtJoin = signal.peerId
        backlog = []
        hostPeerId = ""
        ui.phase = .joining
        signal.request(Envelopes.join(target), expect: ["joined"]) { [weak self] result in
            guard let self else { return }
            self.joining = false
            if self.closed { return }
            switch result {
            case let .success(env):
                self.joinedOn = peerAtJoin
                self.hostPeerId = env.remote
                self.ui.roomId = env.roomId
                let early = self.backlog
                self.backlog = []
                for e in early { self.onEnvelope(e) }
            case let .failure(e):
                if e.fromServer {
                    let phase: RoomPhase = e.message == ServerError.rateLimited ? .rateLimited : .notFound
                    // A room that vanished while getting back in has ended.
                    let ended = self.ui.reconnecting && phase == .notFound
                    self.ui.phase = ended ? .ended : phase
                } else if e.message != "disconnected" {
                    self.ui.phase = .unreachable
                }
            }
        }
    }

    private func onEnvelope(_ env: Envelope) {
        if closed { return }
        switch env.type {
        case "signal":
            if hostPeerId.isEmpty {
                if joining && backlog.count < 100 { backlog.append(env) }
                return
            }
            guard env.from == hostPeerId, let sig = RtcSignals.parse(env.payload) else { return }
            handleSignal(sig)
        case "peer_left":
            if !hostPeerId.isEmpty && env.from == hostPeerId && !halted {
                dropPeer()
                hostPeerId = ""
                ui.phase = .ended
                ui.controlOpen = false
            }
        default:
            break
        }
    }

    private func sendSignal(_ payload: JSON) {
        guard !hostPeerId.isEmpty else { return }
        signal.send(Envelopes.signal(to: hostPeerId, payload: payload))
    }

    private func handleSignal(_ sig: DeviceSignal) {
        switch sig {
        case .pinRequired: onPinRequired(last: nil)
        case let .pinResult(r): onPinResult(r)
        case let .offer(sdp): applyOffer(sdp)
        case let .candidate(c): addRemoteCandidate(c)
        }
    }

    private func onPinRequired(last: PinResult?) {
        if halted { return }
        let roomId = ui.roomId
        // Getting back in after a drop: the token this app got on the way in.
        let token = admittedOnce && !sentToken && !roomId.isEmpty ? passes.get(roomId) : ""
        if !token.isEmpty {
            sentToken = true
            ui.phase = .pin
            ui.pin = PinView(needed: true, busy: true)
            sendSignal(RtcSignals.token(token))
            return
        }
        if !groupKey.isEmpty {
            // A group invitation: the key and the name, on every ask (a
            // reconnection while waiting knocks again; the device never
            // spends a place on a stale knock).
            knocking = true
            ui.phase = .pin
            ui.pin = PinView(needed: true, busy: true)
            sendSignal(RtcSignals.key(groupKey, name: keyName))
            return
        }
        if let pin = pinToSend {
            pinToSend = nil // an invitation's PIN works once
            ui.phase = .pin
            ui.pin = PinView(needed: true, busy: true)
            sendSignal(RtcSignals.pin(pin))
            return
        }
        ui.phase = .pin
        ui.pin = PinView(needed: true, busy: false, last: last)
    }

    private func onPinResult(_ r: PinResult) {
        let roomId = ui.roomId
        if r.ok {
            admittedOnce = true
            sentToken = false
            knocking = false
            ui.knock = .none
            if !r.token.isEmpty && !roomId.isEmpty { passes.save(roomId, r.token) }
            ui.phase = .connecting
            ui.pin = PinView()
            ui.reconnecting = false
            return
        }
        if r.reason == "removed" {
            // The host took this guest out: its token now answers
            // "removed" too, so nothing else is tried. The token is kept:
            // a retry would only hear the same.
            sentToken = false
            stop(.removed)
            return
        }
        if r.reason == "waiting" {
            // The host decides: keep waiting, nothing to type.
            ui.phase = .pin
            ui.pin = PinView(needed: true, busy: false, last: r)
            ui.knock = .waiting
            return
        }
        if sentToken {
            // An old token: forget it and ask the person.
            sentToken = false
            admittedOnce = false
            if !roomId.isEmpty { passes.forget(roomId) }
            onPinRequired(last: nil)
            return
        }
        if knocking {
            // The group invitation did not let this person in: say why, and stop.
            knocking = false
            ui.pin = PinView(needed: true, busy: false, last: r)
            ui.knock = .ended(r.reason.isEmpty ? "wrong" : r.reason)
            stop(.pin)
            return
        }
        onPinRequired(last: r)
    }

    /** Ends the visit for good, leaving the screen on phase: no more joins or reconnections. */
    private func stop(_ phase: RoomPhase) {
        halted = true
        input.releaseAll()
        dropPeer()
        hostPeerId = ""
        ui.phase = phase
        ui.reconnecting = false
        ui.controlOpen = false
        if phase == .removed { ui.pin = PinView() }
        signal.close()
    }

    /** A PIN the person typed on the room's PIN prompt. */
    public func submitPin(_ pin: String) {
        guard Invites.isPin(pin), !hostPeerId.isEmpty else { return }
        ui.pin.busy = true
        sendSignal(RtcSignals.pin(pin))
    }

    /** Relays peer events to the client, dropping those of an old peer. */
    private final class PeerEvents: RtcPeerEvents {
        weak var client: RoomClient?
        let generation: Int

        init(client: RoomClient, generation: Int) {
            self.client = client
            self.generation = generation
        }

        private var current: RoomClient? {
            guard let c = client, c.peerGeneration == generation else { return nil }
            return c
        }

        func onLocalCandidate(_ candidate: IceCandidateInit) { current?.sendSignal(RtcSignals.candidate(candidate)) }
        func onState(_ state: PeerState) { current?.onPeerState(state) }
        func onControlOpen() { current?.onControlOpened() }
        func onControlMessage(_ text: String) { current?.onControl(text) }
    }

    private func ensurePeer() -> RtcPeer {
        if let p = peer { return p }
        peerGeneration += 1
        // STUN/TURN come from signalhub's hello; nothing is hardcoded.
        let p = peers.create(iceServers: signal.iceServers, events: PeerEvents(client: self, generation: peerGeneration))
        peer = p
        remoteSet = false
        return p
    }

    private func applyOffer(_ sdp: String) {
        let p = ensurePeer()
        let gen = peerGeneration
        p.answer(offerSdp: sdp) { [weak self] result in
            guard let self, gen == self.peerGeneration else { return }
            switch result {
            case let .success(answer):
                self.remoteSet = true
                for c in self.pendingCandidates { p.addCandidate(c) }
                self.pendingCandidates = []
                self.sendSignal(RtcSignals.answer(answer))
                if self.ui.phase != .streaming { self.ui.phase = .connecting }
            case .failure:
                self.onPeerState(.failed)
            }
        }
    }

    private func addRemoteCandidate(_ c: IceCandidateInit) {
        if let p = peer, remoteSet {
            p.addCandidate(c)
        } else if pendingCandidates.count < 50 {
            // Before the answer a device sends a handful, never hundreds.
            pendingCandidates.append(c)
        }
    }

    private func onPeerState(_ state: PeerState) {
        switch state {
        case .connected:
            ui.phase = .streaming
            ui.reconnecting = false
        case .failed:
            if !closed && ui.phase != .ended { reconnect() }
        default:
            break
        }
    }

    private func dropPeer() {
        peerGeneration += 1
        peer?.close()
        peer = nil
        remoteSet = false
        pendingCandidates = []
        controlOpen = false
        input.reset()
    }

    private func onControlOpened() {
        controlOpen = true
        sendHello()
        for t in pendingControl { peer?.sendControl(t) }
        pendingControl = []
        ui.controlOpen = true
    }

    private func onControl(_ text: String) {
        guard let m = JSONText.parseObject(text) else { return }
        if m["type"].str(40) == "ping" {
            // The device measures the peer-to-peer round trip with pings.
            peer?.sendControl(JSON.obj(("type", "pong"), ("id", m["id"])).text)
            return
        }
        if m["type"].str(40) == "error" {
            // A refused request: the screen shows a short notice for its code.
            refusals += 1
            ui.refused = Refusal(code: m["code"].str(40), seq: refusals)
            return
        }
        if let s = RoomMessages.parseRoomState(m) {
            ui.room = s
            return
        }
        if let line = RoomMessages.parseChat(m) {
            ui.chat = Array((ui.chat + [line]).suffix(Self.maxChatLines))
            return
        }
        if let who = RoomMessages.parseTyping(m) {
            ui.typing = who
            return
        }
        if let st = RoomMessages.parseStreamStats(m) {
            // The last valid "video" stays (like the website) until a new one arrives.
            ui.stats = StreamStatsView(fps: st.fps ?? ui.stats.fps, aspect: st.aspect ?? ui.stats.aspect, video: st.video ?? ui.stats.video)
        }
    }

    /**
     * Sends a JSON message on "control". Messages sent before the channel
     * opens are kept (up to 20) and flushed on open.
     */
    public func sendControl(_ message: JSON) {
        let text = message.text
        if !controlOpen || peer?.sendControl(text) != true {
            if pendingControl.count < 20 { pendingControl.append(text) }
        }
    }

    private func sendHello() {
        let hello = JSON.obj(("type", "hello"), ("name", .string(name)), ("local_players", .array(localPlayers.map { .int($0) })))
        if controlOpen { peer?.sendControl(hello.text) }
    }

    /** Your name and which local players (touch + controllers) want seats. */
    public func setIdentity(name: String, localPlayers: [Int]) {
        var players = Array(Set(localPlayers.filter { (0..<maxLocalPlayers).contains($0) })).sorted()
        if players.isEmpty { players = [0] }
        // The device's rules (PlayerName): an unusable name is sent empty and the device picks one.
        let clean = PlayerName.sanitize(name)
        if clean == self.name && players == self.localPlayers { return }
        self.name = clean
        self.localPlayers = players
        sendHello()
    }

    public func setPad(player: Int, pad: Pad) { input.setPad(player: player, pad: pad) }

    public func sendChat(_ text: String) {
        let t = String(text.trimmingCharacters(in: .whitespacesAndNewlines).prefix(300))
        if !t.isEmpty { sendControl(.obj(("type", "chat"), ("text", .string(t)))) }
    }

    public func sendTyping(_ on: Bool) { sendControl(.obj(("type", "typing"), ("on", .bool(on)))) }

    public func spectate() { sendControl(.obj(("type", "spectate"))) }

    public func joinQueue() { sendControl(.obj(("type", "queue"))) }

    /** A spectator raises (true) or lowers its hand to ask the host to play. */
    public func raiseHand(_ on: Bool) { sendControl(.obj(("type", "hand"), ("on", .bool(on)))) }

    /**
     * Only the host pauses and resumes a game: a guest asks for a pause
     * (the answer comes as room_state.paused, or a pause_declined chat
     * notice), or withdraws its request with cancel.
     */
    public func requestPause() { sendControl(.obj(("type", "pause_request"))) }

    public func cancelPauseRequest() { sendControl(.obj(("type", "pause_request"), ("cancel", .bool(true)))) }

    public func swapSeat(from: Int, to: Int) { sendControl(.obj(("type", "swap_seat"), ("from", .int(from)), ("to", .int(to)))) }

    public func answerSwap(from: Int, to: Int, accept: Bool) {
        sendControl(.obj(("type", "swap_answer"), ("from", .int(from)), ("to", .int(to)), ("accept", .bool(accept))))
    }
}

/**
 * Sends each local player's controller state on "input": on every change,
 * and every 100 ms while anything is held, because the channel drops lost
 * packets instead of retransmitting. Two extra releases follow a release.
 */
@MainActor
public final class InputSender {
    private final class Player {
        var seq = 0
        var pad = Pad.empty
        var repeatTimer: CancelHandle?
    }

    private let scheduler: Scheduler
    private let send: ([UInt8]) -> Bool
    private var players: [Int: Player] = [:]
    private var epoch = 0

    public init(scheduler: Scheduler, send: @escaping ([UInt8]) -> Bool) {
        self.scheduler = scheduler
        self.send = send
    }

    public func setPad(player: Int, pad: Pad) {
        guard (0..<maxLocalPlayers).contains(player) else { return }
        let p = players[player] ?? Player()
        players[player] = p
        let changed = pad != p.pad
        p.pad = pad
        if changed { sendNow(player) }
        if !pad.isIdle && p.repeatTimer == nil {
            scheduleRepeat(player, p)
        } else if pad.isIdle, let t = p.repeatTimer {
            t.cancel()
            p.repeatTimer = nil
            let e = epoch
            scheduler.after(ms: 50) { [weak self] in
                guard let self, self.epoch == e else { return }
                self.sendNow(player)
                self.scheduler.after(ms: 100) { [weak self] in
                    guard let self, self.epoch == e else { return }
                    self.sendNow(player)
                }
            }
        }
    }

    private func scheduleRepeat(_ player: Int, _ p: Player) {
        p.repeatTimer = scheduler.after(ms: 100) { [weak self, weak p] in
            guard let self, let p, p.repeatTimer != nil else { return }
            self.sendNow(player)
            self.scheduleRepeat(player, p)
        }
    }

    public func releaseAll() {
        for k in players.keys.sorted() { setPad(player: k, pad: .empty) }
    }

    /** Forgets sequences and timers (a new connection starts over). */
    public func reset() {
        epoch += 1
        for p in players.values {
            p.repeatTimer?.cancel()
            p.repeatTimer = nil
        }
        players = [:]
    }

    private func sendNow(_ player: Int) {
        guard let p = players[player] else { return }
        p.seq = (p.seq + 1) & 0xFFFF
        _ = send(InputPacket.encode(seq: p.seq, player: player, pad: p.pad))
    }
}
