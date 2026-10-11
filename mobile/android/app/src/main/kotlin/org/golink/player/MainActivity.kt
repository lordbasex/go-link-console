// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player

import android.content.Intent
import android.hardware.input.InputManager
import android.os.Bundle
import android.provider.Settings
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.core.view.WindowCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import org.golink.player.core.Invites
import org.golink.player.ui.GoLinkTheme
import org.golink.player.ui.HeadphonesScreen
import org.golink.player.ui.HomeScreen
import org.golink.player.ui.IntroScreen
import org.golink.player.ui.JoinScreen
import org.golink.player.ui.Perms
import org.golink.player.ui.RoomScreen
import org.golink.player.ui.ScanScreen
import org.golink.player.ui.SettingsScreen
import org.golink.player.ui.TestControllerScreen
import org.golink.player.ui.Tokens

/** Set once the intro started in this process. */
private var introPlayed = false

class MainActivity : ComponentActivity() {
    private val vm: PlayerViewModel by viewModels()

    private val inputDevices = object : InputManager.InputDeviceListener {
        override fun onInputDeviceAdded(deviceId: Int) = refresh()

        override fun onInputDeviceChanged(deviceId: Int) = refresh()

        override fun onInputDeviceRemoved(deviceId: Int) {
            vm.session?.gamepads?.onDeviceRemoved(deviceId)
            vm.tester.onDeviceRemoved(deviceId)
            refresh()
        }

        private fun refresh() {
            vm.session?.refreshControllers()
            vm.tester.refresh()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        // The first frame is the intro's background (Theme.GoLink.Starting).
        installSplashScreen()
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        if (savedInstanceState == null) handleLink(intent)
        // The intro plays once per process: a cold start, never when the app
        // comes back from the background or the screen rotates.
        // Debug labs (--es lab ...) start without it.
        if (!introPlayed && !(BuildConfig.DEBUG && intent?.getStringExtra("lab") != null)) {
            introPlayed = true
            vm.intro.value = true
        }
        val reducedMotion = Settings.Global.getFloat(contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f
        // Debug builds can slow the intro down to look at it (-e introSlow 4).
        val slow = if (BuildConfig.DEBUG) intent?.getStringExtra("introSlow")?.toFloatOrNull() ?: 1f else 1f
        val lab = if (BuildConfig.DEBUG) intent?.getStringExtra("lab") else null
        getSystemService(InputManager::class.java).registerInputDeviceListener(inputDevices, null)
        setContent {
            GoLinkTheme {
                val intro by vm.intro.collectAsState()
                // Test tags show up as resource ids for UiAutomator (the
                // Android end-to-end test finds the controls by them).
                @OptIn(ExperimentalComposeUiApi::class)
                Box(Modifier.fillMaxSize().background(Tokens.bg).semantics { testTagsAsResourceId = true }) {
                    // Hidden from UiAutomator and TalkBack under the intro.
                    Box(if (intro) Modifier.fillMaxSize().clearAndSetSemantics {} else Modifier.fillMaxSize()) {
                        // Debug builds can open a lab with no room: --es lab alias|overlay|picture|check2x|skin|cinema.
                        when (lab) {
                            "picture" -> org.golink.player.ui.PictureLab(
                                vm.prefs,
                                style = intent?.getStringExtra("style"),
                                bands = intent?.getStringExtra("bands"),
                                startCompare = intent?.getBooleanExtra("compare", false) == true,
                                startSheet = intent?.getBooleanExtra("sheet", false) == true,
                                plain = intent?.getBooleanExtra("plain", false) == true,
                                room = intent?.getStringExtra("room"),
                                up2 = intent?.getBooleanExtra("up2", false) == true,
                                raw = intent?.getBooleanExtra("raw", false) == true,
                            )
                            "check2x" -> org.golink.player.ui.Picture2xCheckScreen()
                            "alias" -> org.golink.player.ui.AliasScreen(saved = vm.prefs.playerName, onEnter = { vm.prefs.playerName = it }, onBack = {})
                            "overlay" -> org.golink.player.ui.OverlayLab()
                            "cinema" -> org.golink.player.ui.CinemaLab()
                            "skin" -> org.golink.player.ui.SkinLab(
                                vm.prefs,
                                skinId = intent?.getStringExtra("skin"),
                                buttons = intent?.getIntExtra("buttons", 6) ?: 6,
                                starts = intent?.getIntExtra("starts", 2) ?: 2,
                                lit = intent?.getIntExtra("lit", 0) ?: 0,
                            )
                            else -> App(vm)
                        }
                    }
                    if (intro) {
                        IntroScreen(
                            sound = vm.prefs.startupSound,
                            reducedMotion = reducedMotion,
                            slow = slow,
                            onDone = { vm.intro.value = false },
                        )
                    }
                }
            }
        }
    }

    override fun onDestroy() {
        getSystemService(InputManager::class.java).unregisterInputDeviceListener(inputDevices)
        super.onDestroy()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleLink(intent)
    }

    /**
     * An App Link (https://go-link.org/g/<invite>) goes straight to the PIN
     * form for that invitation (or the name, for a group invitation's
     * #k=<key>). Only the invitation and that key are read from the link:
     * never a PIN or a signaling server. The key stays in memory.
     */
    private fun handleLink(intent: Intent?) {
        if (intent?.action != Intent.ACTION_VIEW) return
        val uri = intent.data ?: return
        val target = Invites.parseAppLink(uri.scheme, uri.host, uri.path, uri.encodedFragment) ?: return
        vm.openInvite(target)
    }

    // Controllers drive the game while a room is open.
    override fun dispatchKeyEvent(event: KeyEvent): Boolean {
        val session = vm.session
        if (session != null && vm.screen.value == Screen.Room && session.gamepads.onKey(event)) return true
        if (vm.screen.value == Screen.TestController && vm.tester.onKey(event)) return true
        return super.dispatchKeyEvent(event)
    }

    override fun dispatchGenericMotionEvent(event: MotionEvent): Boolean {
        val session = vm.session
        if (session != null && vm.screen.value == Screen.Room && session.gamepads.onMotion(event)) return true
        if (vm.screen.value == Screen.TestController && vm.tester.onMotion(event)) return true
        return super.dispatchGenericMotionEvent(event)
    }

    /** A room is a console: full screen, screen always on. */
    fun setConsoleMode(on: Boolean) {
        val controller = WindowInsetsControllerCompat(window, window.decorView)
        if (on) {
            controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())
            window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        } else {
            controller.show(WindowInsetsCompat.Type.systemBars())
            window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        }
        WindowCompat.getInsetsController(window, window.decorView).isAppearanceLightStatusBars = false
    }
}

@Composable
private fun App(vm: PlayerViewModel) {
    val screen by vm.screen.collectAsState()
    val signal by vm.signal.collectAsState()
    val activity = androidx.compose.ui.platform.LocalContext.current as MainActivity
    LaunchedEffect(screen) { activity.setConsoleMode(screen == Screen.Room) }
    if (screen != Screen.Home) {
        BackHandler {
            if (screen == Screen.Room) vm.leaveRoom() else vm.go(Screen.Home)
        }
    }
    when (val s = screen) {
        Screen.Home -> HomeScreen(
            signal = signal,
            onScan = { vm.go(Screen.Scan) },
            onType = { vm.go(Screen.Join(null)) },
            onInvite = { vm.go(Screen.Join(it)) },
            onSettings = { vm.go(Screen.Settings) },
            onOfficialServer = { vm.setSignal(null) },
            onTestController = { vm.go(Screen.TestController) },
        )
        Screen.Scan -> ScanScreen(
            prefs = vm.prefs,
            onFound = { vm.go(Screen.Join(it)) },
            onType = { vm.go(Screen.Join(null)) },
            onBack = { vm.go(Screen.Home) },
        )
        is Screen.Join -> JoinScreen(
            fixedTarget = s.target,
            termsAccepted = vm.termsAccepted(),
            savedName = vm.prefs.playerName,
            onJoin = { target, pin, name ->
                val bt = Perms.BLUETOOTH
                val ask = bt != null && !Perms.granted(activity, bt) && !vm.prefs.asked(bt)
                vm.join(target, pin, name, askHeadphones = ask)
            },
            onBack = { vm.go(Screen.Home) },
        )
        is Screen.Headphones -> HeadphonesScreen(vm.prefs) { vm.enterRoom(s.target, s.pin, s.name) }
        Screen.Room -> {
            val session = vm.session
            if (session == null) {
                LaunchedEffect(Unit) { vm.go(Screen.Home) }
            } else {
                // A permission granted while in the room changes the route.
                LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { session.reroute() }
                RoomScreen(session, vm.prefs, onLeave = { vm.leaveRoom() }, onAlias = { vm.confirmAlias(it) })
            }
        }
        Screen.Settings -> SettingsScreen(
            name = vm.prefs.playerName,
            signal = signal,
            startupSound = vm.prefs.startupSound,
            onStartupSound = { vm.prefs.startupSound = it },
            onName = { vm.setName(it) },
            onSignal = { vm.setSignal(it) },
            onBack = { vm.go(Screen.Home) },
            skins = org.golink.player.ui.rememberSkinStore(vm.prefs),
        )
        Screen.TestController -> TestControllerScreen(vm.tester, onBack = { vm.go(Screen.Home) }, skins = org.golink.player.ui.rememberSkinStore(vm.prefs))
    }
}
