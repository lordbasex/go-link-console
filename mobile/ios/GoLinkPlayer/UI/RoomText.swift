// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import Foundation
import GoLinkCore

// Text of the room in the reader's language, like the website's
// roomModel.ts and the Android app's RoomText.kt: the device sends chat
// notices as events with values, and names it generated ("Guest 9F3A"),
// which are shown translated.

/** A name as shown: generated guest names in the reader's language. */
func localName(_ name: String) -> String {
    RoomMessages.guestId(name).map { L("guest_name", $0) } ?? name
}

/** A system chat line (the website's systemText). */
func systemText(text: String, event: ChatEvent?, args: ChatArgs?) -> String {
    let n = localName(args?.name ?? "")
    let n2 = localName(args?.name2 ?? "")
    let p = args?.port ?? 0
    let p2 = args?.port2 ?? 0
    switch event {
    case .recordingStarted: return L("ev_recording_started")
    case .recordingStopped: return L("ev_recording_stopped")
    case .gamePaused: return L("ev_paused", n)
    case .gameResumed: return L("ev_resumed", n)
    case .nowWatching: return L("ev_watching", n)
    case .moved: return L("ev_moved", n, p)
    case .swapAsked: return L("ev_swap_asked", n, p, n2, p2)
    case .keptSeat: return L("ev_kept", n, p)
    case .swapped: return L("ev_swapped", n, p, n2, p2)
    case .leftSeat: return L("ev_left", n, p)
    case .seatFree: return L("ev_free", p)
    case .tookSeat: return L("ev_took", n, p)
    case .sentToQueue: return L("ev_sent_to_queue", n, n2, p)
    case .sentToWatch: return L("ev_sent_to_watch", n, n2, p)
    case .letQueue: return L("ev_let_queue", n, n2)
    case .playNow: return L("ev_play_now", n, n2, p)
    case .firstInQueue: return L("ev_first_in_queue", n)
    case .removed: return L("ev_removed", n, n2)
    case .pauseDeclined: return L("ev_pause_declined")
    case .handLater: return L("hand_later")
    case nil: return text
    }
}

/** Why a PIN was refused (the website's pinWrong/pinUsed/pinBlocked/pinLocked). */
func pinRefusal(_ r: PinResult) -> String {
    switch r.reason {
    case "used": return L("pin_used")
    case "blocked": return L("pin_blocked")
    case "locked":
        let minutes = max(1, (r.retryAfter + 59) / 60)
        return L(minutes == 1 ? "pin_locked_one" : "pin_locked_other", minutes)
    default:
        return L(r.left == 1 ? "pin_wrong_one" : "pin_wrong_other", r.left)
    }
}

/** 1 -> "#1" in the reader's language, for the queue. */
func ordinal(_ n: Int) -> String { L("ordinal", n) }

/** Where you stand, as one line. */
func meLine(_ me: Me) -> String {
    switch me {
    case let .player(ports): return L("room_you_player", ports.map { "P\($0)" }.joined(separator: ", "))
    case let .queue(p): return L("room_you_queue", ordinal(p))
    case .spectator: return L("room_you_spectator")
    }
}

/** Why a group invitation did not let this person in (the website's knock.ended). */
func knockEndedText(_ reason: String) -> String {
    switch reason {
    case "declined", "full", "expired", "busy", "blocked", "locked": return L("knock_ended_" + reason)
    default: return L("knock_ended_wrong")
    }
}

/**
 * The notice for a request the device refused (control "error"), or nil
 * for codes this screen does not explain. handAgain is when the next
 * raised hand is allowed (ms since the epoch, 0 = unknown).
 */
func refusalText(_ code: String, handAgain: Double, nowMs: Double = Date().timeIntervalSince1970 * 1000) -> String? {
    switch code {
    case "hand_too_soon":
        let s = Int(((handAgain - nowMs) / 1000).rounded(.up))
        return handAgain > 0 && s > 0 && s <= 60 ? L("hand_again", s) : L("hand_too_soon")
    case "hand_no_host": return L("hand_no_host")
    case "queue_ask_host": return L("hand_ask_host")
    default: return nil
    }
}
