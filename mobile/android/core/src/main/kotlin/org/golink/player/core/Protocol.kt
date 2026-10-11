// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

// Wire format of the signalhub protocol v1. signalhub's README (its own
// repository) is the contract; this mirrors frontend/packages/shared/src/protocol.ts.

object Protocol {
    const val VERSION = "1"

    /** signalhub namespace of this project (its ALLOWED_APPS). */
    const val APP = "go-link-console"

    /** The official signaling server. STUN/TURN are never hardcoded: they come in hello. */
    const val OFFICIAL_SIGNAL_URL = "wss://signal.go-link.org/ws"
}

/** Fixed error texts sent by signalhub, so clients can compare them. */
object ServerError {
    const val INVALID_CODE = "invalid or expired code"
    const val APP_NOT_ALLOWED = "app not allowed"
    const val ROOM_NOT_FOUND = "room not found"
    const val INVALID_INVITE = "invalid or expired invite"
    const val RATE_LIMITED = "rate limit exceeded"
    const val TOO_MANY_MESSAGES = "too many messages"
    const val DEVICE_OFFLINE = "device not connected"
}

/**
 * Adds ?v=1 unless the URL already names a protocol version, like the
 * web's endpoint().
 */
fun endpoint(url: String): String {
    val hasQuery = url.contains('?')
    val query = url.substringAfter('?', "")
    if (query.split('&').any { it == "v" || it.startsWith("v=") }) return url
    return if (hasQuery) "$url&v=${Protocol.VERSION}" else "$url?v=${Protocol.VERSION}"
}

/** One signalhub message (the envelope), as JSON. */
object Envelopes {
    /** join through an invitation link (invite) or a typed 9-digit code. */
    fun join(target: InviteTarget): JsonObject = buildJsonObject {
        put("type", "join")
        put("app", Protocol.APP)
        when (target) {
            is InviteTarget.Link -> put("invite", target.invite)
            is InviteTarget.Code -> put("code", target.code)
        }
    }

    /** A signal to the host device: payload is opaque to signalhub. */
    fun signal(to: String, payload: JsonElement): JsonObject = buildJsonObject {
        put("type", "signal")
        put("to", to)
        put("payload", payload)
    }
}

/** The envelope fields the client reads. */
data class Envelope(
    val type: String,
    val raw: JsonObject,
) {
    val error: String get() = raw["error"].str(200)
    val from: String get() = raw["from"].str(64)
    val remote: String get() = raw["remote"].str(64)
    val roomId: String get() = raw["room_id"].str(64)
    val sessionId: String get() = raw["session_id"].str(64)
    val peerId: String get() = raw["peer_id"].str(64)
    val payload: JsonElement? get() = raw["payload"]

    companion object {
        fun parse(text: String): Envelope? {
            val o = parseObject(text) ?: return null
            val type = (o["type"] as? JsonPrimitive)?.takeIf { it.isString }?.content ?: return null
            return Envelope(type.take(40), o)
        }
    }
}
