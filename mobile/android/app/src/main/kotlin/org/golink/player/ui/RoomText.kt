// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.ui

import android.content.res.Resources
import org.golink.player.R
import org.golink.player.core.ChatEvent
import org.golink.player.core.ChatLine
import org.golink.player.core.DeviceSignal
import org.golink.player.core.RoomMessages

// Text of the room in the reader's language, like the website's
// roomModel.ts: the device sends chat notices as events with values, and
// names it generated ("Guest 9F3A"), which are shown translated.

/** A name as shown: generated guest names in the reader's language. */
fun localName(res: Resources, name: String): String =
    RoomMessages.guestId(name)?.let { res.getString(R.string.guest_name, it) } ?: name

/** A system chat line (the website's systemText). */
fun systemText(res: Resources, line: ChatLine.System): String {
    val a = line.args
    val n = localName(res, a?.name ?: "")
    val n2 = localName(res, a?.name2 ?: "")
    val p = a?.port ?: 0
    val p2 = a?.port2 ?: 0
    return when (line.event) {
        ChatEvent.RECORDING_STARTED -> res.getString(R.string.ev_recording_started)
        ChatEvent.RECORDING_STOPPED -> res.getString(R.string.ev_recording_stopped)
        ChatEvent.GAME_PAUSED -> res.getString(R.string.ev_paused, n)
        ChatEvent.GAME_RESUMED -> res.getString(R.string.ev_resumed, n)
        ChatEvent.NOW_WATCHING -> res.getString(R.string.ev_watching, n)
        ChatEvent.MOVED -> res.getString(R.string.ev_moved, n, p)
        ChatEvent.SWAP_ASKED -> res.getString(R.string.ev_swap_asked, n, p, n2, p2)
        ChatEvent.KEPT_SEAT -> res.getString(R.string.ev_kept, n, p)
        ChatEvent.SWAPPED -> res.getString(R.string.ev_swapped, n, p, n2, p2)
        ChatEvent.LEFT_SEAT -> res.getString(R.string.ev_left, n, p)
        ChatEvent.SEAT_FREE -> res.getString(R.string.ev_free, p)
        ChatEvent.TOOK_SEAT -> res.getString(R.string.ev_took, n, p)
        ChatEvent.SENT_TO_QUEUE -> res.getString(R.string.ev_sent_to_queue, n, n2, p)
        ChatEvent.SENT_TO_WATCH -> res.getString(R.string.ev_sent_to_watch, n, n2, p)
        ChatEvent.LET_QUEUE -> res.getString(R.string.ev_let_queue, n, n2)
        ChatEvent.PLAY_NOW -> res.getString(R.string.ev_play_now, n, n2, p)
        ChatEvent.FIRST_IN_QUEUE -> res.getString(R.string.ev_first_in_queue, n)
        ChatEvent.REMOVED -> res.getString(R.string.ev_removed, n, n2)
        ChatEvent.PAUSE_DECLINED -> res.getString(R.string.ev_pause_declined)
        ChatEvent.HAND_LATER -> res.getString(R.string.hand_later)
        null -> line.text
    }
}

/** Why a PIN was refused (the website's pinWrong/pinUsed/pinBlocked/pinLocked). */
fun pinRefusal(res: Resources, r: DeviceSignal.PinResult): String = when (r.reason) {
    "used" -> res.getString(R.string.pin_used)
    "blocked" -> res.getString(R.string.pin_blocked)
    "locked" -> res.getQuantityString(R.plurals.pin_locked, maxOf(1, (r.retryAfter + 59) / 60), maxOf(1, (r.retryAfter + 59) / 60))
    else -> res.getQuantityString(R.plurals.pin_wrong, r.left, r.left)
}

/**
 * A request the device refused, by its error code (the website's hand
 * notes); null for codes this app does not explain. handAgain is when the
 * next raised hand is allowed (ms since the epoch, 0 = unknown).
 */
fun refusalText(res: Resources, code: String, handAgain: Long, now: Long = System.currentTimeMillis()): String? = when (code) {
    "hand_too_soon" -> {
        val s = ((handAgain - now + 999) / 1000).toInt()
        if (handAgain > 0 && s in 1..60) res.getString(R.string.hand_again, s) else res.getString(R.string.hand_too_soon)
    }
    "hand_no_host" -> res.getString(R.string.hand_no_host)
    "queue_ask_host" -> res.getString(R.string.hand_ask_host)
    else -> null
}

/** Why a group invitation's knock ended (the website's knock.ended texts). */
fun knockEndText(res: Resources, reason: String): String = res.getString(
    when (reason) {
        "declined" -> R.string.knock_end_declined
        "full" -> R.string.knock_end_full
        "expired" -> R.string.knock_end_expired
        "busy" -> R.string.knock_end_busy
        "blocked" -> R.string.knock_end_blocked
        "locked" -> R.string.knock_end_locked
        else -> R.string.knock_end_wrong
    },
)

/** 1 -> "1st"... in the reader's language, for the queue. */
fun ordinal(res: Resources, n: Int): String = res.getString(R.string.ordinal, n)
