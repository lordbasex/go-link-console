// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import GoLinkCore
import QuartzCore
@preconcurrency import WebRTC

/**
 * Reads the numbers of the room's stats overlay from libwebrtc's
 * statistics report: the inbound video (frames decoded, size, codec),
 * the packets of the video and the game sound, the game sound's codec,
 * and the candidate pair in use (round trip and candidate types).
 * GoLinkCore's LiveStatsMeter turns successive samples into rates.
 */
enum LiveStatsReader {
    static func sample(_ report: RTCStatisticsReport, labels: [String: String]) -> RtcSample {
        let all = report.statistics
        func num(_ v: NSObject?) -> Double? { (v as? NSNumber)?.doubleValue }
        func int64(_ v: NSObject?) -> Int64? { num(v).map { Int64($0) } }
        func codec(_ s: RTCStatistics) -> RTCStatistics? { (s.values["codecId"] as? String).flatMap { all[$0] } }

        var out = RtcSample(atMs: CACurrentMediaTime() * 1000)
        var received: Int64 = 0
        var lost: Int64 = 0
        var counted = false
        var gameAudio: RTCStatistics?
        for s in all.values where s.type == "inbound-rtp" {
            let m = s.values
            let kind = m["kind"] as? String
            let label = (m["trackIdentifier"] as? String).flatMap { labels[$0] }
            if kind == "video" {
                out.framesDecoded = int64(m["framesDecoded"])
                out.frameWidth = num(m["frameWidth"]).map { Int($0) }
                out.frameHeight = num(m["frameHeight"]).map { Int($0) }
                out.videoMime = codec(s)?.values["mimeType"] as? String
            } else if kind == "audio" {
                // The game sound, not the players' voices.
                if label == "game" || (label == nil && gameAudio == nil) { gameAudio = s } else { continue }
            } else {
                continue
            }
            received += int64(m["packetsReceived"]) ?? 0
            lost += max(0, int64(m["packetsLost"]) ?? 0)
            counted = true
        }
        if counted {
            out.packetsReceived = received
            out.packetsLost = lost
        }
        if let a = gameAudio, let c = codec(a) {
            out.audioMime = c.values["mimeType"] as? String
            out.audioClockRate = num(c.values["clockRate"]).map { Int($0) }
        }
        // The pair in use: the transport's selected one, or the nominated pair that succeeded.
        let selectedId = all.values.first { $0.type == "transport" }.flatMap { $0.values["selectedCandidatePairId"] as? String }
        let pair = selectedId.flatMap { all[$0] } ?? all.values.first {
            $0.type == "candidate-pair" && ($0.values["nominated"] as? NSNumber)?.boolValue == true && ($0.values["state"] as? String) == "succeeded"
        }
        if let pair {
            out.rttSeconds = num(pair.values["currentRoundTripTime"])
            out.localCandidateType = (pair.values["localCandidateId"] as? String).flatMap { all[$0]?.values["candidateType"] as? String }
            out.remoteCandidateType = (pair.values["remoteCandidateId"] as? String).flatMap { all[$0]?.values["candidateType"] as? String }
        }
        return out
    }

    /**
     * Each voice slot's level now (slot 1-4 -> 0 to 1, linear), from the
     * inbound audio of the voice tracks: who speaks lights up, without
     * touching the audio itself (the website reads the same level from
     * RTCRtpReceiver's synchronization sources).
     */
    static func voiceLevels(_ report: RTCStatisticsReport, labels: [String: String]) -> [Int: Double] {
        var out: [Int: Double] = [:]
        for s in report.statistics.values where s.type == "inbound-rtp" && (s.values["kind"] as? String) == "audio" {
            guard let label = (s.values["trackIdentifier"] as? String).flatMap({ labels[$0] }),
                  let slot = RtcSignals.voicePort(label) else { continue }
            out[slot] = (s.values["audioLevel"] as? NSNumber)?.doubleValue ?? 0
        }
        return out
    }
}
