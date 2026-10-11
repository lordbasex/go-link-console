// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import GoLinkCore
@preconcurrency import WebRTC

/** Media the device sends, handed to the room screen. Called on the main actor. */
@MainActor
protocol MediaListener: AnyObject {
    func onVideo(_ track: RTCVideoTrack)
    func onGameAudio(_ track: RTCAudioTrack)
    /**
     * One of the four voice tracks (slot 1-4), forwarded by the device: it
     * carries whoever speaks now (room_state.voice_slots says who).
     */
    func onVoice(slot: Int, track: RTCAudioTrack)
    /** The connection is closing: its tracks go away. */
    func onMediaClosed()
}

/**
 * The app's side of the WebRTC link with the device, the iOS counterpart
 * of the web's HostStream (frontend/packages/shared/src/stream.ts) and the
 * Android app's AndroidRtcPeer: it answers the device's offer, receives the
 * game's video and sound and the others' voices, and speaks the
 * "control" and "input" channels.
 */
@MainActor
final class IOSRtcPeer: NSObject, RtcPeer {
    private let events: RtcPeerEvents
    private weak var media: MediaListener?
    private var pc: RTCPeerConnection!
    private var control: RTCDataChannel?
    private var input: RTCDataChannel?
    private var micSender: RTCRtpSender?
    private var micTrack: RTCAudioTrack?
    private var closed = false

    /** What each received track carries, by track id: "video", "game" or "voice-pN". */
    private(set) var trackLabels: [String: String] = [:]

    init(engine: WebRtcEngine, iceServers: [IceServer], events: RtcPeerEvents, media: MediaListener) {
        self.events = events
        self.media = media
        super.init()
        let config = RTCConfiguration()
        // STUN/TURN from signalhub's hello, never hardcoded.
        config.iceServers = iceServers.map { RTCIceServer(urlStrings: $0.urls, username: $0.username, credential: $0.credential) }
        config.sdpSemantics = .unifiedPlan
        config.continualGatheringPolicy = .gatherContinually
        let constraints = RTCMediaConstraints(mandatoryConstraints: nil, optionalConstraints: nil)
        pc = engine.factory.peerConnection(with: config, constraints: constraints, delegate: self)
    }

    func answer(offerSdp: String, done: @escaping (Result<String, Error>) -> Void) {
        guard let pc, !closed else {
            done(.failure(PeerError.closed))
            return
        }
        pc.setRemoteDescription(RTCSessionDescription(type: .offer, sdp: offerSdp)) { [weak self] error in
            DispatchQueue.main.async {
                guard let self, !self.closed else { return done(.failure(PeerError.closed)) }
                if let error { return done(.failure(error)) }
                self.prepareMicrophone(offerSdp)
                let constraints = RTCMediaConstraints(mandatoryConstraints: nil, optionalConstraints: nil)
                pc.answer(for: constraints) { answer, error in
                    DispatchQueue.main.async {
                        guard let answer, error == nil else { return done(.failure(error ?? PeerError.noAnswer)) }
                        pc.setLocalDescription(answer) { error in
                            DispatchQueue.main.async {
                                if let error { done(.failure(error)) } else { done(.success(answer.sdp)) }
                            }
                        }
                    }
                }
            }
        }
    }

    /** The device offers the microphone line as recvonly: answer it sendonly. */
    private func prepareMicrophone(_ offer: String) {
        guard let mid = RtcSignals.micMid(offer), let t = pc.transceivers.first(where: { $0.mid == mid }) else { return }
        var error: NSError?
        t.setDirection(.sendOnly, error: &error)
        micSender = t.sender
        if let micTrack { t.sender.track = micTrack }
    }

    /**
     * Sets the microphone (nil stops sending). It swaps the track on the
     * line the device reserved for it, so nothing is renegotiated.
     */
    func setMicTrack(_ track: RTCAudioTrack?) {
        micTrack = track
        if !closed { micSender?.track = track }
    }

    func addCandidate(_ candidate: IceCandidateInit) {
        guard !closed else { return }
        pc.add(RTCIceCandidate(sdp: candidate.candidate, sdpMLineIndex: Int32(candidate.sdpMLineIndex), sdpMid: candidate.sdpMid)) { _ in }
    }

    func sendControl(_ text: String) -> Bool {
        guard !closed, let ch = control, ch.readyState == .open, let data = text.data(using: .utf8) else { return false }
        return ch.sendData(RTCDataBuffer(data: data, isBinary: false))
    }

    func sendInput(_ packet: [UInt8]) -> Bool {
        guard !closed, let ch = input, ch.readyState == .open else { return false }
        return ch.sendData(RTCDataBuffer(data: Data(packet), isBinary: true))
    }

    /** libwebrtc's statistics (RTP counters, audio levels), for the debug probe. */
    func stats(_ done: @escaping (RTCStatisticsReport) -> Void) {
        guard !closed else { return }
        pc.statistics { report in DispatchQueue.main.async { done(report) } }
    }

    func close() {
        guard !closed else { return }
        closed = true
        media?.onMediaClosed()
        control?.delegate = nil
        control?.close()
        input?.close()
        control = nil
        input = nil
        micSender = nil
        pc.delegate = nil
        let old = pc
        // Closing waits on libwebrtc's threads: never on the main thread.
        DispatchQueue.global(qos: .utility).async { old?.close() }
    }

    enum PeerError: Error { case closed, noAnswer }
}

extension IOSRtcPeer: RTCPeerConnectionDelegate {
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didGenerate candidate: RTCIceCandidate) {
        let c = IceCandidateInit(candidate: candidate.sdp, sdpMid: candidate.sdpMid, sdpMLineIndex: Int(candidate.sdpMLineIndex))
        DispatchQueue.main.async {
            if !self.closed { self.events.onLocalCandidate(c) }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCPeerConnectionState) {
        DispatchQueue.main.async {
            guard !self.closed else { return }
            switch newState {
            case .connected: self.events.onState(.connected)
            case .failed: self.events.onState(.failed)
            default: break
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didOpen dataChannel: RTCDataChannel) {
        DispatchQueue.main.async {
            guard !self.closed else { return }
            switch dataChannel.label {
            case "input":
                self.input = dataChannel
            case "control":
                self.control = dataChannel
                dataChannel.delegate = self
                if dataChannel.readyState == .open { self.events.onControlOpen() }
            default:
                dataChannel.close() // "files" is only for the host's own browser
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didAdd rtpReceiver: RTCRtpReceiver, streams mediaStreams: [RTCMediaStream]) {
        let id = mediaStreams.first?.streamId ?? ""
        let track = rtpReceiver.track
        DispatchQueue.main.async {
            guard !self.closed, let track else { return }
            if let v = track as? RTCVideoTrack {
                self.trackLabels[v.trackId] = "video"
                self.media?.onVideo(v)
            } else if let a = track as? RTCAudioTrack {
                let slot = RtcSignals.voicePort(id)
                self.trackLabels[a.trackId] = slot.map { "voice-p\($0)" } ?? "game"
                if let slot { self.media?.onVoice(slot: slot, track: a) } else { self.media?.onGameAudio(a) }
            }
        }
    }

    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange stateChanged: RTCSignalingState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didAdd stream: RTCMediaStream) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didRemove stream: RTCMediaStream) {}
    nonisolated func peerConnectionShouldNegotiate(_ peerConnection: RTCPeerConnection) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCIceConnectionState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didChange newState: RTCIceGatheringState) {}
    nonisolated func peerConnection(_ peerConnection: RTCPeerConnection, didRemove candidates: [RTCIceCandidate]) {}
}

extension IOSRtcPeer: RTCDataChannelDelegate {
    nonisolated func dataChannelDidChangeState(_ dataChannel: RTCDataChannel) {
        let open = dataChannel.readyState == .open
        DispatchQueue.main.async {
            if open && !self.closed && dataChannel === self.control { self.events.onControlOpen() }
        }
    }

    nonisolated func dataChannel(_ dataChannel: RTCDataChannel, didReceiveMessageWith buffer: RTCDataBuffer) {
        guard !buffer.isBinary, buffer.data.count <= 256 * 1024, let text = String(data: buffer.data, encoding: .utf8) else { return }
        DispatchQueue.main.async {
            if !self.closed { self.events.onControlMessage(text) }
        }
    }
}
