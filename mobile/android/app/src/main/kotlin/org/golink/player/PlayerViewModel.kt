// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.golink.player.core.InviteTarget
import org.golink.player.core.SignalUrls
import org.golink.player.core.Terms
import org.golink.player.input.ControllerTester
import java.time.Instant

/** The app's screens. */
sealed interface Screen {
    data object Home : Screen

    data object Scan : Screen

    /**
     * The PIN form. target is fixed when it came from a QR code or a link
     * (the person only types the PIN), or null to type the code too.
     */
    data class Join(val target: InviteTarget?) : Screen

    /** Asks for the headphones permission (Android 12+) before a room. name: a group invitation's knock name. */
    data class Headphones(val target: InviteTarget, val pin: String, val name: String = "") : Screen

    data object Room : Screen

    data object Settings : Screen

    /** "Test controller": the pad and real controllers, offline. */
    data object TestController : Screen
}

class PlayerViewModel(app: Application) : AndroidViewModel(app) {
    val prefs = Prefs(app)

    private val _screen = MutableStateFlow<Screen>(Screen.Home)
    val screen: StateFlow<Screen> = _screen.asStateFlow()

    private val _signal = MutableStateFlow(prefs.signal)
    val signal: StateFlow<SignalUrls.Choice> = _signal.asStateFlow()

    var session: RoomSession? = null
        private set

    /** The input of the "Test controller" screen. */
    val tester = ControllerTester(app)

    /** The startup intro, shown once per process (a cold start). */
    val intro = MutableStateFlow(false)

    fun go(screen: Screen) {
        _screen.value = screen
    }

    /** An invitation from a QR code or an App Link: straight to the PIN. */
    fun openInvite(target: InviteTarget) {
        leaveRoom()
        _screen.value = Screen.Join(target)
    }

    fun termsAccepted(): Boolean = Terms.accepted(prefs)

    /**
     * Joins with the typed PIN, or with a group invitation's key and the
     * name the host sees, after the terms were accepted on the form.
     */
    fun join(target: InviteTarget, pin: String, name: String, askHeadphones: Boolean) {
        Terms.accept(prefs, Instant.now().toString())
        // The knock's name is the player's name from now on.
        if (name.isNotEmpty()) prefs.playerName = name
        if (askHeadphones) {
            _screen.value = Screen.Headphones(target, pin, name)
            return
        }
        enterRoom(target, pin, name)
    }

    fun enterRoom(target: InviteTarget, pin: String, name: String = "") {
        leaveRoom()
        val s = RoomSession(getApplication(), prefs, target, pin, viewModelScope, knockName = name)
        session = s
        s.start()
        _screen.value = Screen.Room
    }

    fun leaveRoom() {
        session?.close()
        session = null
        if (_screen.value == Screen.Room) _screen.value = Screen.Home
    }

    fun setName(name: String) {
        prefs.playerName = name
        session?.setName(prefs.playerName)
    }

    /** The alias step before the room: saves the name and sends it. */
    fun confirmAlias(name: String) {
        prefs.playerName = name
        session?.confirmAlias(prefs.playerName)
    }

    /** Saves a custom signaling server (already tested), or null for the official one. */
    fun setSignal(url: String?) {
        prefs.saveCustomSignal(url)
        _signal.value = prefs.signal
    }

    override fun onCleared() {
        session?.close()
        session = null
    }
}
