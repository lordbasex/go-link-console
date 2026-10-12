// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** Small string storage the platform provides (SharedPreferences, UserDefaults...). */
interface KeyValueStore {
    fun get(key: String): String?

    fun set(key: String, value: String?)
}

/**
 * The return token the device gives on the way into a room, kept per
 * room_id like the web's localStorage "go-link.room-passes". It is only
 * used to get back in automatically after a dropped connection in the same
 * session; a new join always asks for the invitation's PIN. Room ids change
 * with every session of a room, so only the last few are kept.
 */
class RoomPasses(private val store: KeyValueStore) {
    fun get(roomId: String): String = read()[roomId] ?: ""

    fun save(roomId: String, token: String) {
        val passes = LinkedHashMap(read())
        passes.remove(roomId)
        passes[roomId] = token // newest last
        while (passes.size > MAX) passes.remove(passes.keys.first())
        write(passes)
    }

    fun forget(roomId: String) {
        val passes = LinkedHashMap(read())
        if (passes.remove(roomId) != null) write(passes)
    }

    private fun read(): Map<String, String> {
        val o = store.get(KEY)?.let { parseObject(it) } ?: return emptyMap()
        val out = LinkedHashMap<String, String>()
        for ((k, v) in o) {
            val s = v.strOrNull() ?: continue
            out[k] = s
        }
        return out
    }

    private fun write(passes: Map<String, String>) {
        store.set(KEY, JsonObject(passes.mapValues { JsonPrimitive(it.value) }).toString())
    }

    companion object {
        const val KEY = "go-link.room-passes"
        const val MAX = 20
    }
}

/** The terms of use every player accepts before joining (docs/legal.md). */
object Terms {
    /** Must equal TERMS_VERSION in frontend/apps/web/src/legal.ts (a test checks it). */
    const val VERSION = "2026-10-11"
    const val TERMS_URL = "https://console.go-link.org/terms"
    const val PRIVACY_URL = "https://console.go-link.org/privacy"
    const val STORE_KEY = "go-link.terms"

    fun accepted(store: KeyValueStore): Boolean {
        val o = store.get(STORE_KEY)?.let { parseObject(it) } ?: return false
        return o["version"].strOrNull() == VERSION
    }

    fun accept(store: KeyValueStore, isoNow: String) {
        store.set(STORE_KEY, JsonObject(mapOf("version" to JsonPrimitive(VERSION), "at" to JsonPrimitive(isoNow))).toString())
    }
}
