// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

/**
 * What an invitation points at: the 22-character invite of a
 * https://go-link.org/g/<invite> link (and its QR code), or the room's
 * 9-digit code. The PIN is never part of either: the person types it.
 */
sealed interface InviteTarget {
    /**
     * key is a group invitation's key from the link's fragment
     * (/g/<invite>#k=<key>), or "": with it the person knocks with a name
     * instead of typing a PIN. It stays in memory only (never logged or
     * saved), so toString leaves it out.
     */
    data class Link(val invite: String, val key: String = "") : InviteTarget {
        val group: Boolean get() = key.isNotEmpty()

        override fun toString(): String = "Link(invite=$invite, group=$group)"
    }

    data class Code(val code: String) : InviteTarget

    /** The code grouped like the web shows it: "123 456 789". */
    fun display(): String = when (this) {
        is Link -> "https://${Invites.HOST}/g/$invite"
        is Code -> "${code.substring(0, 3)} ${code.substring(3, 6)} ${code.substring(6)}"
    }
}

object Invites {
    /** The only site whose invitation links the app follows. */
    const val HOST = "go-link.org"

    private val INVITE = Regex("^[A-Za-z0-9_-]{22}$")
    private val LINK = Regex("^https://go-link\\.org/g/([A-Za-z0-9_-]{22})/?(?:#([^#\\s]*))?$")
    private val KEY = Regex("^[A-Za-z0-9_-]{43}$")
    private val CODE = Regex("^\\d{9}$")

    /** Largest text worth looking at: a QR code can carry kilobytes. */
    private const val MAX_INPUT = 200

    /**
     * Reads a scanned QR code. Only two things are accepted: exactly an
     * https://go-link.org/g/<invite> link, or a 9-digit code. Anything
     * else (another site, a signaling server, query parameters) is
     * rejected, so a hostile QR can never point the app elsewhere. From
     * the link's fragment only a group invitation's key (k) is read.
     */
    fun parseScanned(text: String): InviteTarget? {
        if (text.length > MAX_INPUT) return null
        val t = text.trim()
        parseLink(t)?.let { return it }
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
        parseLink(t)?.let { return it }
        if (INVITE.matches(t)) return InviteTarget.Link(t)
        return parseCode(t)
    }

    /**
     * Reads an Android App Link: scheme https, host go-link.org, path
     * /g/<invite>. The query is ignored; from the fragment only a group
     * invitation's key is read.
     */
    fun parseAppLink(scheme: String?, host: String?, path: String?, fragment: String? = null): InviteTarget? {
        if (scheme != "https" || host != HOST || path == null) return null
        val m = Regex("^/g/([A-Za-z0-9_-]{22})/?$").matchEntire(path) ?: return null
        if (fragment != null && fragment.length > MAX_INPUT) return InviteTarget.Link(m.groupValues[1])
        return InviteTarget.Link(m.groupValues[1], groupKeyOf(fragment ?: ""))
    }

    /**
     * The key of a group invitation from a link's fragment ("k=<key>",
     * among other "&" separated values, like the web's groupKeyOf): exactly
     * 43 base64url characters, or "" for anything else.
     */
    fun groupKeyOf(fragment: String): String {
        val v = fragment.removePrefix("#").split('&').firstOrNull { it.startsWith("k=") }?.substring(2) ?: return ""
        return if (KEY.matches(v)) v else ""
    }

    private fun parseLink(t: String): InviteTarget? {
        val m = LINK.matchEntire(t) ?: return null
        return InviteTarget.Link(m.groupValues[1], groupKeyOf(m.groupValues[2]))
    }

    private fun parseCode(t: String): InviteTarget? {
        val digits = t.replace(Regex("[\\s.-]"), "")
        return if (CODE.matches(digits)) InviteTarget.Code(digits) else null
    }

    /** A PIN is exactly 6 digits. */
    fun isPin(pin: String): Boolean = pin.length == 6 && pin.all { it in '0'..'9' }
}
