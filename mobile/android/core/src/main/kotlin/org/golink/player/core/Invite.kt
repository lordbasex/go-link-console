// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

/**
 * What an invitation points at: the 22-character invite of a
 * https://play.go-link.org/g/<invite> link (and its QR code), or the room's
 * 9-digit code. The PIN is never part of either: the person types it.
 */
sealed interface InviteTarget {
    data class Link(val invite: String) : InviteTarget

    data class Code(val code: String) : InviteTarget

    /** The code grouped like the web shows it: "123 456 789". */
    fun display(): String = when (this) {
        is Link -> "https://${Invites.HOST}/g/$invite"
        is Code -> "${code.substring(0, 3)} ${code.substring(3, 6)} ${code.substring(6)}"
    }
}

object Invites {
    /** The only site whose invitation links the app follows. */
    const val HOST = "play.go-link.org"

    private val INVITE = Regex("^[A-Za-z0-9_-]{22}$")
    private val LINK = Regex("^https://play\\.go-link\\.org/g/([A-Za-z0-9_-]{22})/?$")
    private val CODE = Regex("^\\d{9}$")

    /** Largest text worth looking at: a QR code can carry kilobytes. */
    private const val MAX_INPUT = 200

    /**
     * Reads a scanned QR code. Only two things are accepted: exactly an
     * https://play.go-link.org/g/<invite> link, or a 9-digit code. Anything
     * else (another site, a signaling server, a PIN, parameters) is
     * rejected, so a hostile QR can never point the app elsewhere.
     */
    fun parseScanned(text: String): InviteTarget? {
        if (text.length > MAX_INPUT) return null
        val t = text.trim()
        LINK.matchEntire(t)?.let { return InviteTarget.Link(it.groupValues[1]) }
        return parseCode(t)
    }

    /**
     * Reads what a person typed or pasted: the link (as strict as a QR
     * code), the bare 22-character invite, or the 9-digit code (spaces,
     * dots and dashes allowed, as the web shows it: "123 456 789").
     */
    fun parseTyped(text: String): InviteTarget? {
        if (text.length > MAX_INPUT) return null
        val t = text.trim()
        LINK.matchEntire(t)?.let { return InviteTarget.Link(it.groupValues[1]) }
        if (INVITE.matches(t)) return InviteTarget.Link(t)
        return parseCode(t)
    }

    /**
     * Reads an Android App Link: scheme https, host play.go-link.org, path
     * /g/<invite>. Query and fragment are ignored, never used.
     */
    fun parseAppLink(scheme: String?, host: String?, path: String?): InviteTarget? {
        if (scheme != "https" || host != HOST || path == null) return null
        val m = Regex("^/g/([A-Za-z0-9_-]{22})/?$").matchEntire(path) ?: return null
        return InviteTarget.Link(m.groupValues[1])
    }

    private fun parseCode(t: String): InviteTarget? {
        val digits = t.replace(Regex("[\\s.-]"), "")
        return if (CODE.matches(digits)) InviteTarget.Code(digits) else null
    }

    /** A PIN is exactly 6 digits. */
    fun isPin(pin: String): Boolean = pin.length == 6 && pin.all { it in '0'..'9' }
}
