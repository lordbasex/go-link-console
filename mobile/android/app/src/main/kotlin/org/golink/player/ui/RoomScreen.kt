// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.ui

import androidx.compose.runtime.snapshotFlow
import androidx.compose.foundation.text.input.maxLength
import androidx.compose.foundation.text.input.InputTransformation
import androidx.compose.foundation.text.input.TextFieldLineLimits
import androidx.compose.foundation.text.input.clearText
import androidx.compose.foundation.text.input.rememberTextFieldState
import android.content.res.Configuration
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.isImeVisible
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.clickable
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Chat
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Gamepad
import androidx.compose.material.icons.filled.Headphones
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.MicOff
import androidx.compose.material.icons.filled.PanTool
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.People
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.golink.player.Prefs
import org.golink.player.R
import org.golink.player.RoomSession
import org.golink.player.core.ChatEvent
import org.golink.player.core.ChatLine
import org.golink.player.core.GameControls
import org.golink.player.core.Invites
import org.golink.player.core.Me
import org.golink.player.core.RoomPhase
import org.golink.player.core.RoomStateView
import org.golink.player.core.RoomUi
import org.golink.player.core.startButtonCount
import org.golink.player.core.PictureSettings
import org.golink.player.picture.PictureParams

/**
 * The room: the game's picture with the on-screen gamepad around it, like
 * the website's console mode. Portrait is a Game Boy (picture on top, pad
 * below); landscape is a Switch (pad halves on both sides). Chat, seats
 * and the queue live in a sheet opened from the dock; the gear under the
 * gamepad button opens Game settings (name, sound, picture, stats) without
 * leaving the room. The picture is drawn by the picture renderer
 * (PictureDrawer) over the whole screen area, sides included.
 */
@Composable
fun RoomScreen(session: RoomSession, prefs: Prefs, onLeave: () -> Unit, onAlias: (String) -> Unit) {
    val ui by session.client.ui.collectAsState()
    val video by session.video.collectAsState()
    val sound by session.sound.collectAsState()
    val controllers by session.controllers.collectAsState()
    val connected by session.connected.collectAsState()
    val controllerBits by session.controllerButtons.collectAsState()
    val aliasDone by session.aliasDone.collectAsState()
    val liveStats by session.liveStats.collectAsState()
    val output by session.audioOutput.collectAsState()
    var sheet by rememberSaveable { mutableStateOf<SheetTab?>(null) }
    var touchOn by rememberSaveable { mutableStateOf(prefs.touchPad) }
    // With a real controller, the gamepad button shows the pad as a see-through display of it.
    var padWithController by rememberSaveable { mutableStateOf(false) }
    var statsOn by rememberSaveable { mutableStateOf(prefs.statsOverlay) }
    // The highest refresh rate while in the room (90/120/144 Hz), and the measured one for the stats.
    val screenHz by rememberHighRefreshRate()
    LaunchedEffect(statsOn) { session.showStats(statsOn) }
    var micAsk by remember { mutableStateOf(false) }
    // Game settings: open, and whether it opened at the sound part (the players' Sound button).
    var settingsOpen by rememberSaveable { mutableStateOf(false) }
    var settingsAtSound by rememberSaveable { mutableStateOf(false) }
    // How this phone draws the game: the viewer's own choice (remembered)
    // wins, then the room's default from its host, then the app's.
    var savedPicture by remember { mutableStateOf(prefs.savedPicture) }
    var pictureOk by remember { mutableStateOf(true) }
    var compare by rememberSaveable { mutableStateOf(false) }
    var split by rememberSaveable { mutableStateOf(0.5) }
    val density = LocalDensity.current.density.toDouble()
    // "Remove animations": the ambient light changes more slowly, like the website's prefers-reduced-motion.
    val resolver = LocalContext.current.contentResolver
    val reducedMotion = remember { android.provider.Settings.Global.getFloat(resolver, android.provider.Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f }
    val snackbar = remember { SnackbarHostState() }
    // The gamepad's skin (Game settings › Skin); null is the classic pad.
    val skins = rememberSkinStore(prefs)
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val resources = context.resources
    LaunchedEffect(session) {
        // A chosen headset or microphone disconnected: say so, once.
        session.audioLost.collect { snackbar.showSnackbar(lostText(resources, it)) }
    }
    // The host said no to this player's pause request, or "not now" to a
    // raised hand: a notice in the chat and here.
    var seenLine by remember { mutableStateOf(ui.chat.lastOrNull()) }
    LaunchedEffect(ui.chat) {
        val from = seenLine?.let { ui.chat.lastIndexOf(it) + 1 } ?: 0
        val fresh = ui.chat.drop(from)
        seenLine = ui.chat.lastOrNull()
        if (fresh.any { it is ChatLine.System && it.event == ChatEvent.PAUSE_DECLINED }) {
            snackbar.showSnackbar(resources.getString(R.string.ev_pause_declined))
        }
        if (fresh.any { it is ChatLine.System && it.event == ChatEvent.HAND_LATER }) {
            snackbar.currentSnackbarData?.dismiss()
            snackbar.showSnackbar(resources.getString(R.string.hand_later))
        }
    }
    // A request the device refused: why, in a short notice.
    LaunchedEffect(ui.refused) {
        val text = ui.refused?.let { refusalText(resources, it.code, ui.room?.you?.handAgain ?: 0) } ?: return@LaunchedEffect
        snackbar.currentSnackbarData?.dismiss()
        snackbar.showSnackbar(text)
    }
    // "Do you want to join the voice?": asked each time this phone becomes
    // able to talk (it joins as a player or in the queue, or is moved there
    // from watching) while its microphone is off. Not now keeps listening.
    val talker = ui.room?.canTalk == true && ui.room?.voice != false
    var voiceAsk by remember { mutableStateOf(false) }
    var wasTalker by remember { mutableStateOf(false) }
    LaunchedEffect(talker) {
        if (talker && !wasTalker && !sound.micOn) voiceAsk = true
        if (!talker) voiceAsk = false
        wasTalker = talker
    }
    LaunchedEffect(sound.micOn) { if (sound.micOn) voiceAsk = false }
    val landscape = LocalConfiguration.current.orientation == Configuration.ORIENTATION_LANDSCAPE
    val room = ui.room
    val picture = PictureSettings.resolve(savedPicture, room?.picture)
    val controls = room?.controls ?: GameControls.DEFAULT
    val pad = rememberTouchPad { session.setTouchButtons(it) }
    pad.fourWay = controls.control == "joy4way"
    val streaming = ui.phase == RoomPhase.STREAMING
    val myPorts = (room?.me as? Me.Player)?.ports ?: emptyList()
    val starts = startButtonCount(controls.players, room?.seated ?: 1)
    val aspect = (ui.stats.aspect ?: (4.0 / 3.0)).toFloat()
    // With a controller in hand, the on-screen pad steps aside; the gamepad
    // button can bring it back see-through, showing what the controller presses.
    val hasController = controllers.isNotEmpty() || connected.isNotEmpty()
    val displayOnly = hasController
    val showPad = streaming && if (hasController) padWithController else touchOn
    pad.extra = if (displayOnly) {
        // The controller's Start is this player's own start button on the panel.
        controllerBits or (if (controllerBits and org.golink.player.core.Button.START != 0) myPorts.fold(0) { a, p -> a or org.golink.player.core.startOf(p) } else 0)
    } else {
        0
    }
    val controllerName = controllers.firstOrNull()?.name ?: connected.firstOrNull()?.name ?: ""
    val padAlpha = if (displayOnly) 0.55f else 1f
    // The activity handles rotations itself (configChanges), so a rotation
    // only swaps the layout: let go of every held button (the device gets
    // 0) whenever the layout changes or the pad goes away.
    DisposableEffect(landscape, showPad, displayOnly) { onDispose { pad.releaseAll() } }

    val dockIn: @Composable (Boolean, Boolean) -> Unit = { vertical, capsule ->
        Dock(
            ui = ui,
            session = session,
            touchOn = if (hasController) padWithController else touchOn,
            onTouch = {
                if (hasController) {
                    padWithController = !padWithController
                } else {
                    touchOn = !touchOn
                    prefs.touchPad = touchOn
                }
            },
            onSheet = { sheet = it },
            settingsOpen = settingsOpen,
            onSettings = {
                settingsAtSound = false
                settingsOpen = true
            },
            onMicAsk = { micAsk = true },
            headset = output != org.golink.player.audio.AudioRouter.Output.SPEAKER,
            vertical = vertical,
            onExplain = { text ->
                scope.launch {
                    snackbar.currentSnackbarData?.dismiss()
                    snackbar.showSnackbar(text ?: resources.getString(R.string.room_pause_asked))
                }
            },
            capsule = capsule,
        )
    }
    val dock: @Composable () -> Unit = { dockIn(landscape, false) }
    val screen: @Composable (Modifier) -> Unit = { m ->
        // "picture": the video's own view is a platform view that UiAutomator cannot name.
        Box(m.background(Tokens.video).testTag("picture"), contentAlignment = Alignment.Center) {
            // The renderer covers the whole area: it letterboxes the game at its
            // display aspect (never cropped) and draws the sides.
            VideoView(
                video,
                session.eglContext,
                PictureParams(picture, if (compare && pictureOk) split else null, aspect.toDouble(), density, reducedMotion, ui.stats.video?.native),
                Modifier.fillMaxSize().testTag("video"),
                onUnavailable = { pictureOk = false },
            )
            if (streaming && compare && pictureOk) {
                CompareDivider(split, { split = it }, styleName(picture.style))
            }
            Overlay(ui, onLeave, onRetry = { session.client.reconnect() }, onPin = { session.client.submitPin(it) })
            if (streaming) {
                StatsCorner(
                    on = statsOn,
                    stats = liveStats,
                    video = ui.stats.video,
                    screenHz = screenHz,
                    onToggle = { statsOn = !statsOn; prefs.statsOverlay = statsOn },
                    modifier = Modifier.align(Alignment.TopStart).padding(8.dp),
                )
            }
            if (streaming && room != null) {
                Column(Modifier.align(Alignment.TopEnd).padding(8.dp), horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (room.recording) RecBadge(Modifier)
                }
                // Top right, round and see-through like the stats button (under the REC badge while recording).
                if (hasController) {
                    // In cinema mode the floating capsule owns the right edge: stay clear of it.
                    val end = LocalScreenTrailingInset.current
                    Box(Modifier.align(Alignment.TopEnd).padding(top = if (room.recording) 40.dp else 8.dp, end = 8.dp + end)) { ControllerChip(controllerName) }
                }
                if (room.paused) {
                    Surface(color = Color(0xCC05060A), shape = RoundedCornerShape(12.dp)) {
                        Text(
                            stringResource(R.string.room_paused_by, localName(LocalContext.current.resources, room.pausedBy)),
                            color = Tokens.text,
                            modifier = Modifier.padding(16.dp),
                        )
                    }
                }
                Column(Modifier.align(Alignment.BottomCenter).padding(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (voiceAsk && aliasDone) {
                        VoiceAsk(
                            onYes = {
                                voiceAsk = false
                                session.setGameMuted(false) // talking implies listening
                                if (Perms.granted(context, Perms.MIC)) session.setMic(true) else micAsk = true
                            },
                            onNo = { voiceAsk = false },
                        )
                    }
                    SwapOffers(room, session, Modifier)
                }
                if (room.you.pauseAsked != null && !room.paused) {
                    PauseAskedBanner(
                        onCancel = { session.client.cancelPauseRequest() },
                        modifier = Modifier.align(Alignment.TopCenter).padding(top = if (statsOn) (if (ui.stats.video == null) 114.dp else if (ui.stats.video?.quality == null) 132.dp else 150.dp) else 52.dp, start = 8.dp, end = 8.dp),
                    )
                }
            }
        }
    }

    // The alias step: once per visit, when the room first lets this player in.
    val admitted = ui.phase == RoomPhase.CONNECTING || ui.phase == RoomPhase.STREAMING
    val showAlias = !aliasDone && admitted
    Box(Modifier.fillMaxSize().background(Tokens.bg).then(if (showAlias) Modifier.clearAndSetSemantics {} else Modifier)) {
        val skin = skins.selected
        if (showPad && skin != null) {
            SkinConsole(
                skin = skin,
                picture = skins.background(skin, landscape),
                landscape = landscape,
                pad = pad,
                controls = controls,
                starts = starts,
                myPorts = myPorts,
                aspect = aspect.toDouble(),
                displayOnly = displayOnly,
                keepDock = sheet != null || settingsOpen,
                header = { compact -> TopBar(ui, onLeave, compact = compact) },
                screen = screen,
                dock = { vertical -> dockIn(vertical, true) },
            )
        } else if (landscape && !showPad) {
            // Cinema: the picture as large as the screen allows (its sides style fills the rest), the
            // room's buttons and the leave button in a floating capsule that folds away after 3 s.
            var wake by remember { mutableIntStateOf(0) }
            BoxWithConstraints(Modifier.fillMaxSize().safeDrawingPadding()) {
                androidx.compose.runtime.CompositionLocalProvider(LocalScreenTrailingInset provides 64.dp) {
                screen(
                    Modifier.fillMaxSize().pointerInput(Unit) {
                        // Watches taps on the picture without taking them from its overlays.
                        awaitEachGesture {
                            awaitFirstDown(requireUnconsumed = false, pass = PointerEventPass.Initial)
                            wake++
                        }
                    },
                )
                }
                FloatingCapsule(
                    keep = sheet != null || settingsOpen,
                    wake = wake,
                    maxHeight = maxHeight - 16.dp,
                    modifier = Modifier.align(Alignment.CenterEnd).padding(end = 8.dp),
                ) {
                    TopBar(ui, onLeave, compact = true)
                    Box(Modifier.width(28.dp).height(1.dp).background(Color.White.copy(alpha = 0.14f)))
                    dockIn(true, true)
                }
            }
        } else if (landscape) {
            Row(Modifier.fillMaxSize().safeDrawingPadding()) {
                if (showPad) {
                    PadSurface(pad, Modifier.fillMaxHeight().weight(0.26f).alpha(padAlpha), enabled = !displayOnly) {
                        Column(
                            Modifier.fillMaxSize().padding(8.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.SpaceEvenly,
                        ) {
                            TopBar(ui, onLeave, compact = true)
                            DPad(pad, 150.dp)
                            PillButton(pad, org.golink.player.core.Button.COIN, stringResource(R.string.room_coin), tag = "pad-coin")
                        }
                    }
                } else {
                    Column(Modifier.fillMaxHeight().width(64.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                        TopBar(ui, onLeave, compact = true)
                    }
                }
                screen(Modifier.fillMaxHeight().weight(if (showPad) 0.48f else 1f))
                if (showPad) {
                    PadSurface(pad, Modifier.fillMaxHeight().weight(0.26f), enabled = !displayOnly) {
                        Row(Modifier.fillMaxSize()) {
                            Column(
                                Modifier.fillMaxHeight().weight(1f).padding(8.dp).alpha(padAlpha),
                                horizontalAlignment = Alignment.CenterHorizontally,
                                verticalArrangement = Arrangement.SpaceEvenly,
                            ) {
                                StartRow(pad, starts, myPorts, withCoin = false)
                                ActionButtons(pad, controls, 64.dp)
                            }
                            dock()
                        }
                    }
                } else {
                    dock()
                }
            }
        } else {
            Column(Modifier.fillMaxSize().safeDrawingPadding()) {
                TopBar(ui, onLeave, compact = false)
                screen(Modifier.fillMaxWidth().aspectRatio(aspect.coerceIn(1f, 2f)))
                dock()
                if (showPad) {
                    PadSurface(pad, Modifier.fillMaxWidth().weight(1f).alpha(padAlpha), enabled = !displayOnly) {
                        Column(Modifier.fillMaxSize().padding(horizontal = 16.dp, vertical = 8.dp)) {
                            Row(
                                Modifier.fillMaxWidth().weight(1f),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween,
                            ) {
                                DPad(pad, 160.dp)
                                Box(Modifier.widthIn(max = 200.dp)) { ActionButtons(pad, controls, 64.dp) }
                            }
                            Box(Modifier.fillMaxWidth().padding(bottom = 8.dp), contentAlignment = Alignment.Center) {
                                StartRow(pad, starts, myPorts)
                            }
                            Text(
                                "go-link",
                                color = Tokens.faint.copy(alpha = 0.5f),
                                fontWeight = FontWeight.Bold,
                                fontSize = 12.sp,
                                modifier = Modifier.align(Alignment.CenterHorizontally),
                            )
                        }
                    }
                } else {
                    // No pad: the chat fills the space under the picture.
                    Box(Modifier.fillMaxWidth().weight(1f).padding(8.dp)) {
                        ChatPanel(ui, session, Modifier.fillMaxSize())
                    }
                }
            }
        }
        SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).safeDrawingPadding().padding(12.dp)) { data ->
            Snackbar(data, shape = RoundedCornerShape(50), containerColor = Tokens.surface2, contentColor = Tokens.text)
        }
    }

    if (showAlias) {
        AliasScreen(saved = prefs.playerName, onEnter = onAlias, onBack = onLeave)
        return
    }

    if (micAsk) {
        MicExplainer(prefs, session, onDone = { micAsk = false })
    }
    sheet?.let { tab ->
        RoomSheet(
            ui,
            session,
            tab,
            onTab = { sheet = it },
            onSound = {
                sheet = null
                settingsAtSound = true
                settingsOpen = true
            },
            onClose = { sheet = null },
        )
    }
    if (settingsOpen) {
        GameSettingsSheet(
            landscape = landscape,
            onClose = { settingsOpen = false },
            picture = PictureChoice(
                picture = picture,
                onPicture = {
                    prefs.choosePicture(it)
                    savedPicture = prefs.savedPicture
                },
                available = pictureOk,
                compare = compare,
                onCompare = { compare = it },
                roomDefault = room?.picture,
                chosen = savedPicture.chosen,
                onUseRoomDefault = {
                    prefs.clearPicture()
                    savedPicture = prefs.savedPicture
                },
            ),
            statsOn = statsOn,
            onStats = {
                statsOn = it
                prefs.statsOverlay = it
            },
            name = {
                NameSection(prefs.playerName) {
                    prefs.playerName = it
                    session.setName(prefs.playerName)
                }
            },
            sound = { SoundSection(session) },
            scrollToSound = settingsAtSound,
            skins = skins,
        )
    }
}

enum class SheetTab { CHAT, PLAYERS }

/** Room on the right of the picture kept free for the floating capsule (cinema mode). */
val LocalScreenTrailingInset = androidx.compose.runtime.compositionLocalOf { 0.dp }

@Composable
private fun TopBar(ui: RoomUi, onLeave: () -> Unit, compact: Boolean) {
    val res = LocalContext.current.resources
    Row(
        Modifier.then(if (compact) Modifier else Modifier.fillMaxWidth().padding(horizontal = 4.dp)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconButton(onClick = onLeave, modifier = Modifier.size(48.dp)) {
            Icon(Icons.Filled.Close, contentDescription = stringResource(R.string.room_leave), tint = Tokens.text2)
        }
        if (!compact) {
            val info = ui.room?.info
            Column(Modifier.weight(1f)) {
                Text(
                    info?.title?.ifEmpty { null } ?: info?.game?.ifEmpty { null } ?: stringResource(R.string.app_name),
                    color = Tokens.text,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                val me = ui.room?.me
                val line = when (me) {
                    is Me.Player -> stringResource(R.string.room_you_player, me.ports.joinToString(", ") { "P$it" })
                    is Me.Queue -> stringResource(R.string.room_you_queue, ordinal(res, me.position))
                    Me.Spectator -> stringResource(R.string.room_you_spectator)
                    null -> info?.host?.ifEmpty { null }?.let { stringResource(R.string.room_host, it) } ?: ""
                }
                if (line.isNotEmpty()) Text(line, color = Tokens.muted, fontSize = 12.sp, maxLines = 1, modifier = Modifier.testTag("room-seat"))
            }
        }
    }
}

@Composable
private fun RecBadge(modifier: Modifier) {
    Surface(modifier, color = Color(0xCC05060A), shape = RoundedCornerShape(50), border = BorderStroke(1.dp, Tokens.rec)) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(Tokens.rec, CircleShape))
            Spacer(Modifier.width(6.dp))
            Text(stringResource(R.string.room_rec), color = Tokens.text, fontSize = 12.sp, fontWeight = FontWeight.Bold)
        }
    }
}

/** Joining, the PIN, connecting, or why the room is not available. */
@Composable
private fun Overlay(ui: RoomUi, onLeave: () -> Unit, onRetry: () -> Unit, onPin: (String) -> Unit) {
    val text = when (ui.phase) {
        RoomPhase.JOINING -> stringResource(if (ui.reconnecting) R.string.room_reconnecting else R.string.room_joining)
        RoomPhase.CONNECTING -> stringResource(R.string.room_connecting)
        RoomPhase.PIN -> if (!ui.pin.busy && ui.pin.needed) null else stringResource(R.string.room_checking_pin)
        else -> null
    }
    if (text != null) {
        Surface(color = Color(0xCC05060A), shape = RoundedCornerShape(16.dp)) {
            Column(Modifier.padding(20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                CircularProgressIndicator(color = Tokens.accent, modifier = Modifier.size(32.dp))
                Spacer(Modifier.height(12.dp))
                Text(text, color = Tokens.text, textAlign = TextAlign.Center)
            }
        }
        return
    }
    if (ui.phase == RoomPhase.PIN && ui.pin.needed && !ui.pin.busy) {
        PinPrompt(ui, onPin, onLeave)
        return
    }
    when (ui.phase) {
        RoomPhase.WAITING -> {
            // A group invitation's knock: the host decides.
            FinalCard(
                title = stringResource(R.string.knock_waiting_title),
                text = stringResource(R.string.knock_waiting_text),
                button = stringResource(R.string.knock_leave),
                onLeave = onLeave,
                hand = true,
                tag = "room-knock-waiting",
            )
            return
        }
        RoomPhase.REFUSED -> {
            FinalCard(
                title = stringResource(R.string.knock_ended_title),
                text = knockEndText(LocalContext.current.resources, ui.knockEnd),
                button = stringResource(R.string.knock_back),
                onLeave = onLeave,
                tag = "room-knock-ended",
            )
            return
        }
        RoomPhase.REMOVED -> {
            FinalCard(
                title = stringResource(R.string.removed_title),
                text = stringResource(R.string.removed_text),
                button = stringResource(R.string.room_leave),
                onLeave = onLeave,
                tag = "room-removed",
            )
            return
        }
        else -> Unit
    }
    val problem = when (ui.phase) {
        RoomPhase.ENDED -> R.string.room_ended
        RoomPhase.NOT_FOUND -> R.string.room_not_found
        RoomPhase.RATE_LIMITED -> R.string.room_rate_limited
        RoomPhase.UNREACHABLE -> R.string.room_unreachable
        else -> null
    } ?: return
    Surface(color = Tokens.surface, shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, Tokens.border), modifier = Modifier.padding(16.dp)) {
        Column(Modifier.padding(18.dp).widthIn(max = 420.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(stringResource(problem), color = Tokens.text, modifier = Modifier.testTag("room-problem"))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (ui.phase == RoomPhase.RATE_LIMITED || ui.phase == RoomPhase.UNREACHABLE) {
                    PrimaryButton(stringResource(R.string.room_retry), onRetry)
                }
                SecondaryButton(stringResource(R.string.room_leave), onLeave)
            }
        }
    }
}

/** A card over the picture with a title, a text and one way out (waiting for the host, or a final answer). */
@Composable
private fun FinalCard(title: String, text: String, button: String, onLeave: () -> Unit, tag: String, hand: Boolean = false) {
    Surface(color = Tokens.surface, shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, Tokens.border), modifier = Modifier.padding(16.dp).testTag(tag)) {
        Column(Modifier.padding(18.dp).widthIn(max = 420.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            if (hand) Text("\u270B", fontSize = 28.sp, modifier = Modifier.clearAndSetSemantics {})
            Text(title, color = Tokens.text, fontWeight = FontWeight.SemiBold, fontSize = 18.sp)
            Text(text, color = Tokens.muted, fontSize = 14.sp)
            SecondaryButton(button, onLeave, Modifier.fillMaxWidth().testTag("$tag-leave"))
        }
    }
}

/** "Do you want to join the voice?", a small card under the picture: it never blocks the game. */
@Composable
private fun VoiceAsk(onYes: () -> Unit, onNo: () -> Unit) {
    Surface(
        color = Tokens.surface,
        shape = RoundedCornerShape(14.dp),
        border = BorderStroke(1.dp, Tokens.accentTintBorder),
        modifier = Modifier.testTag("voice-ask"),
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Filled.Mic, contentDescription = null, tint = Tokens.accent, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.voice_ask_title), color = Tokens.text, fontSize = 14.sp)
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PrimaryButton(stringResource(R.string.voice_ask_yes), onYes, Modifier.testTag("voice-ask-yes"))
                SecondaryButton(stringResource(R.string.voice_ask_no), onNo, Modifier.testTag("voice-ask-no"))
            }
        }
    }
}

/** The room asks again: the PIN the person typed failed (or a return token expired). */
@Composable
private fun PinPrompt(ui: RoomUi, onPin: (String) -> Unit, onLeave: () -> Unit) {
    val res = LocalContext.current.resources
    var pin by remember { mutableStateOf("") }
    val last = ui.pin.last
    val stop = last?.reason == "blocked" || last?.reason == "locked"
    Surface(color = Tokens.surface, shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, Tokens.border), modifier = Modifier.padding(12.dp)) {
        Column(Modifier.padding(16.dp).widthIn(max = 380.dp).imePadding(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(stringResource(R.string.pin_prompt_title), color = Tokens.text, fontWeight = FontWeight.SemiBold, fontSize = 18.sp)
            if (last != null) Notice(pinRefusal(res, last), danger = true, modifier = Modifier.testTag("room-pin-error")) else Text(stringResource(R.string.pin_prompt_text), color = Tokens.muted, fontSize = 14.sp)
            if (!stop) {
                OutlinedTextField(
                    value = pin,
                    onValueChange = { v -> pin = v.filter { it.isDigit() }.take(6) },
                    singleLine = true,
                    placeholder = { Text("000000") },
                    textStyle = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 22.sp, letterSpacing = 6.sp),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Go),
                    keyboardActions = KeyboardActions(onGo = { if (Invites.isPin(pin)) onPin(pin) }),
                    colors = OutlinedTextFieldDefaults.colors(focusedBorderColor = Tokens.accent, cursorColor = Tokens.accent),
                    modifier = Modifier.fillMaxWidth().testTag("room-pin"),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (!stop) PrimaryButton(stringResource(R.string.pin_send), { onPin(pin) }, Modifier.testTag("room-pin-send"), enabled = Invites.isPin(pin))
                SecondaryButton(stringResource(R.string.room_leave), onLeave)
            }
        }
    }
}

/** Requests to swap controllers waiting for this player's answer. */
@Composable
private fun SwapOffers(room: RoomStateView, session: RoomSession, modifier: Modifier) {
    val res = LocalContext.current.resources
    val offer = room.you.swapOffers.firstOrNull() ?: return
    Surface(modifier, color = Tokens.surface, shape = RoundedCornerShape(14.dp), border = BorderStroke(1.dp, Tokens.accentTintBorder)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(stringResource(R.string.room_swap_offer, localName(res, offer.name), offer.from, offer.to), color = Tokens.text, fontSize = 14.sp)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PrimaryButton(stringResource(R.string.room_accept), { session.client.answerSwap(offer.from, offer.to, true) })
                SecondaryButton(stringResource(R.string.room_decline), { session.client.answerSwap(offer.from, offer.to, false) })
            }
        }
    }
}

/** The room's buttons: voice, sound, pause, chat, players, on-screen pad and Game settings. */
@Composable
private fun Dock(
    ui: RoomUi,
    session: RoomSession,
    touchOn: Boolean,
    onTouch: () -> Unit,
    onSheet: (SheetTab) -> Unit,
    settingsOpen: Boolean,
    onSettings: () -> Unit,
    onMicAsk: () -> Unit,
    headset: Boolean,
    vertical: Boolean,
    onExplain: (String?) -> Unit,
    /** Inside a skin's menu capsule: only the buttons, the capsule lays them out. */
    capsule: Boolean = false,
) {
    val sound by session.sound.collectAsState()
    val context = LocalContext.current
    val micGranted by rememberGranted(Perms.MIC)
    val room = ui.room
    val seated = room?.me is Me.Player
    val voiceOn = room?.voice != false
    // The players and the queue talk (the device drops a spectator's voice).
    val canTalk = room?.canTalk == true && voiceOn
    val items: List<@Composable () -> Unit> = listOf(
        {
            DockButton(
                icon = if (sound.micOn && canTalk) Icons.Filled.Mic else Icons.Filled.MicOff,
                label = stringResource(
                    when {
                        !voiceOn -> R.string.room_voice_off
                        room?.canTalk != true -> R.string.room_voice_players_only
                        !micGranted -> R.string.room_mic_blocked
                        sound.micOn -> R.string.room_mic_on
                        else -> R.string.room_mic_off
                    },
                ),
                on = sound.micOn && canTalk,
                enabled = canTalk,
                tag = "dock-mic",
            ) {
                when {
                    sound.micOn -> session.setMic(false)
                    Perms.granted(context, Perms.MIC) -> session.setMic(true)
                    else -> onMicAsk()
                }
            }
        },
        {
            DockButton(
                icon = if (sound.gameMuted) Icons.AutoMirrored.Filled.VolumeOff else if (headset) Icons.Filled.Headphones else Icons.AutoMirrored.Filled.VolumeUp,
                label = stringResource(if (sound.gameMuted) R.string.room_sound_off else R.string.room_sound_on),
                on = !sound.gameMuted,
                tag = "dock-sound",
            ) { session.setGameMuted(!sound.gameMuted) }
        },
        {
            // A spectator raises a hand to ask the host to play (only while the host is in the room).
            if (room != null && room.me is Me.Spectator) {
                val up = room.you.hand
                val label = stringResource(if (up) R.string.hand_lower else R.string.hand_raise)
                val noHost = stringResource(R.string.hand_no_host)
                DockButton(
                    icon = Icons.Filled.PanTool,
                    label = label,
                    on = up,
                    dimmed = !up && !room.hostOnline,
                    tag = "dock-hand",
                ) {
                    if (!up && !room.hostOnline) onExplain(noHost) else session.client.raiseHand(!up)
                }
            }
            // Only the host pauses: a seated player asks for a pause.
            if (room != null && seated) {
                val asked = room.you.pauseAsked != null
                val why = when {
                    room.paused -> R.string.room_paused_host
                    asked -> R.string.room_pause_cancel
                    !room.pausable -> R.string.room_pause_not_pausable
                    !room.hostOnline -> R.string.room_pause_host_away
                    else -> R.string.room_pause_ask
                }
                val label = stringResource(why)
                val can = room.paused || asked || (room.pausable && room.hostOnline)
                DockButton(
                    icon = if (asked) Icons.Filled.Close else Icons.Filled.Pause,
                    label = label,
                    on = room.paused || asked,
                    dimmed = !can || room.paused,
                    tag = "dock-pause",
                ) {
                    when {
                        room.paused || !can -> onExplain(label)
                        asked -> session.client.cancelPauseRequest()
                        else -> {
                            session.client.requestPause()
                            onExplain(null)
                        }
                    }
                }
            }
        },
        {
            DockButton(Icons.AutoMirrored.Filled.Chat, stringResource(R.string.room_chat), on = false, badge = ui.chat.isNotEmpty() && room?.chat != false, tag = "dock-chat") {
                onSheet(SheetTab.CHAT)
            }
        },
        { DockButton(Icons.Filled.People, stringResource(R.string.room_players), on = false, tag = "dock-players") { onSheet(SheetTab.PLAYERS) } },
        { DockButton(Icons.Filled.Gamepad, stringResource(R.string.room_touchpad), on = touchOn, tag = "dock-pad", onClick = onTouch) },
        { DockButton(Icons.Filled.Settings, stringResource(R.string.room_settings), on = settingsOpen, tag = "dock-settings", onClick = onSettings) },
    )
    if (capsule) {
        items.forEach { it() }
    } else if (vertical) {
        Column(
            Modifier.fillMaxHeight().padding(2.dp),
            verticalArrangement = Arrangement.spacedBy(2.dp, Alignment.CenterVertically),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) { items.forEach { it() } }
    } else {
        Row(
            // Up to seven 48 dp buttons: 352 dp, so it fits a 360 dp phone.
            Modifier.fillMaxWidth().padding(horizontal = 2.dp, vertical = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(2.dp, Alignment.CenterHorizontally),
            verticalAlignment = Alignment.CenterVertically,
        ) { items.forEach { it() } }
    }
}

@Composable
internal fun DockButton(
    icon: ImageVector,
    label: String,
    on: Boolean,
    enabled: Boolean = true,
    badge: Boolean = false,
    tag: String = "",
    /** Looks disabled but still takes a tap (to explain why). */
    dimmed: Boolean = false,
    onClick: () -> Unit,
) {
    val style = LocalDockStyle.current
    val (skinFill, skinIcon) = style?.let { dockColors(it, on) } ?: (Color.Unspecified to Color.Unspecified)
    Box(if (tag.isEmpty()) Modifier else Modifier.testTag(tag)) {
        IconButton(
            onClick = onClick,
            enabled = enabled,
            modifier = Modifier.size(48.dp),
            colors = IconButtonDefaults.iconButtonColors(
                containerColor = if (dimmed) Tokens.sunken else if (style != null) skinFill else if (on) Tokens.accentTint else Tokens.surface,
                contentColor = if (dimmed) Tokens.faint.copy(alpha = 0.6f) else if (style != null) skinIcon else if (on) Tokens.accent else Tokens.text2,
                disabledContainerColor = Tokens.sunken,
                disabledContentColor = Tokens.faint.copy(alpha = 0.6f),
            ),
        ) {
            Icon(icon, contentDescription = label, modifier = Modifier.size(22.dp))
        }
        if (badge) Box(Modifier.align(Alignment.TopEnd).padding(8.dp).size(8.dp).background(Tokens.accent, CircleShape))
    }
}

/** The microphone explainer, then the system prompt; denied means play and listen only. */
@Composable
private fun MicExplainer(prefs: Prefs, session: RoomSession, onDone: () -> Unit) {
    val context = LocalContext.current
    var denied by remember { mutableStateOf(false) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { ok ->
        prefs.markAsked(Perms.MIC)
        if (ok) {
            session.setMic(true)
            onDone()
        } else {
            denied = true
        }
    }
    androidx.compose.ui.window.Dialog(onDismissRequest = onDone) {
        Surface(color = Tokens.surface, shape = RoundedCornerShape(20.dp), border = BorderStroke(1.dp, Tokens.border)) {
            PermissionExplainer(
                icon = Icons.Filled.Mic,
                title = stringResource(R.string.perm_mic_title),
                text = stringResource(R.string.perm_mic_text),
                allow = stringResource(if (denied) R.string.perm_open_settings else R.string.perm_allow),
                notNow = stringResource(R.string.perm_not_now),
                note = if (denied) stringResource(R.string.perm_mic_denied) else null,
                onAllow = {
                    if (denied) {
                        Perms.openAppSettings(context)
                        onDone()
                    } else {
                        launcher.launch(Perms.MIC)
                    }
                },
                onNotNow = onDone,
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class, androidx.compose.foundation.layout.ExperimentalLayoutApi::class)
@Composable
private fun RoomSheet(ui: RoomUi, session: RoomSession, tab: SheetTab, onTab: (SheetTab) -> Unit, onSound: () -> Unit, onClose: () -> Unit) {
    val state = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    // Typing on a phone held sideways: the keyboard takes most of the screen, so the
    // sheet keeps only the messages and the field, at the full height left (with the
    // handle, the tabs and 85 % of the screen, the field was squeezed out of sight).
    val typing = WindowInsets.isImeVisible &&
        LocalConfiguration.current.orientation == android.content.res.Configuration.ORIENTATION_LANDSCAPE
    ModalBottomSheet(
        onDismissRequest = onClose,
        sheetState = state,
        containerColor = Tokens.surface,
        dragHandle = if (typing) null else { { androidx.compose.material3.BottomSheetDefaults.DragHandle() } },
    ) {
        // The sheet is its own window: its test tags need their own flag.
        @OptIn(androidx.compose.ui.ExperimentalComposeUiApi::class)
        Column(Modifier.fillMaxWidth().then(if (typing) Modifier.fillMaxHeight() else Modifier.fillMaxHeight(0.85f)).imePadding().semantics { testTagsAsResourceId = true }) {
            if (!typing) {
                PrimaryTabRow(selectedTabIndex = tab.ordinal, containerColor = Tokens.surface, contentColor = Tokens.accent) {
                    Tab(selected = tab == SheetTab.CHAT, onClick = { onTab(SheetTab.CHAT) }, text = { Text(stringResource(R.string.room_chat)) })
                    Tab(selected = tab == SheetTab.PLAYERS, onClick = { onTab(SheetTab.PLAYERS) }, text = { Text(stringResource(R.string.room_players)) })
                }
            }
            when (tab) {
                SheetTab.CHAT -> ChatPanel(ui, session, Modifier.fillMaxSize().padding(12.dp))
                SheetTab.PLAYERS -> PlayersPanel(ui, session, onSound, Modifier.fillMaxSize().padding(12.dp))
            }
        }
    }
}

@Composable
fun ChatPanel(ui: RoomUi, session: RoomSession, modifier: Modifier) {
    val res = LocalContext.current.resources
    val room = ui.room
    // TextFieldState keeps every typed character even while the room screen
    // recomposes many times a second (a String value dropped fast input).
    val input = rememberTextFieldState()
    var typingSent by remember { mutableLongStateOf(0L) }
    LaunchedEffect(input) {
        snapshotFlow { input.text.toString() }.collect { text ->
            val now = System.currentTimeMillis()
            if (text.isNotBlank() && now - typingSent > 2_500) {
                typingSent = now
                session.client.sendTyping(true)
            } else if (text.isBlank() && typingSent != 0L) {
                typingSent = 0L
                session.client.sendTyping(false)
            }
        }
    }
    val list = rememberLazyListState()
    LaunchedEffect(ui.chat.size) { if (ui.chat.isNotEmpty()) list.animateScrollToItem(ui.chat.size - 1) }
    val myName = room?.you?.name
    if (!ui.controlOpen) return
    Column(modifier) {
        if (room?.chat == false) {
            Notice(stringResource(R.string.room_chat_off))
            return@Column
        }
        LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            items(ui.chat) { line ->
                when (line) {
                    is ChatLine.System -> Text(systemText(res, line), color = Tokens.faint, fontSize = 13.sp, modifier = Modifier.fillMaxWidth(), textAlign = TextAlign.Center)
                    is ChatLine.User -> {
                        val mine = line.name == myName
                        Column(Modifier.fillMaxWidth(), horizontalAlignment = if (mine) Alignment.End else Alignment.Start) {
                            Text(
                                localName(res, line.name) + (line.port?.let { " · P$it" } ?: ""),
                                color = line.port?.let { Tokens.player(it) } ?: Tokens.muted,
                                fontSize = 12.sp,
                                fontWeight = FontWeight.SemiBold,
                            )
                            Surface(color = if (mine) Tokens.accentTint else Tokens.surface2, shape = RoundedCornerShape(12.dp)) {
                                Text(line.text, color = Tokens.text, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                            }
                        }
                    }
                }
            }
        }
        if (ui.typing.isNotEmpty()) {
            Text(
                stringResource(R.string.room_typing, ui.typing.joinToString(", ") { localName(res, it.name) }),
                color = Tokens.muted,
                fontSize = 12.sp,
                modifier = Modifier.padding(vertical = 4.dp),
            )
        }
        val send = {
            val text = input.text.toString()
            if (text.isNotBlank()) {
                session.client.sendChat(text)
                input.clearText()
                typingSent = 0L // the device clears "typing" with the message
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                state = input,
                placeholder = { Text(stringResource(R.string.room_chat_hint)) },
                lineLimits = TextFieldLineLimits.SingleLine,
                inputTransformation = InputTransformation.maxLength(300),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Send),
                onKeyboardAction = { send() },
                colors = OutlinedTextFieldDefaults.colors(focusedBorderColor = Tokens.accent, cursorColor = Tokens.accent),
                shape = RoundedCornerShape(50),
                modifier = Modifier.weight(1f).testTag("chat-input"),
            )
            IconButton(onClick = send, enabled = input.text.isNotBlank(), modifier = Modifier.size(48.dp).testTag("chat-send")) {
                Icon(Icons.AutoMirrored.Filled.Send, contentDescription = stringResource(R.string.room_send), tint = Tokens.accent)
            }
        }
    }
}

@Composable
private fun PlayersPanel(ui: RoomUi, session: RoomSession, onSound: () -> Unit, modifier: Modifier) {
    val res = LocalContext.current.resources
    val room = ui.room ?: return
    val sound by session.sound.collectAsState()
    val myPort = (room.me as? Me.Player)?.ports?.firstOrNull()
    // Voices are silenced by person (a voice track carries whoever speaks now).
    val voices = room.voice && sound.voices.isNotEmpty()
    LazyColumn(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
        items(room.seats.size) { i ->
            val port = i + 1
            val seat = room.seats[i]
            Surface(color = Tokens.surface2, shape = RoundedCornerShape(12.dp), border = BorderStroke(1.dp, if (seat?.you == true) Tokens.accentTintBorder else Tokens.border)) {
                Row(Modifier.fillMaxWidth().padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(32.dp).background(Tokens.player(port), CircleShape), contentAlignment = Alignment.Center) {
                        Text("P$port", color = Tokens.onAccent, fontSize = 11.sp, fontWeight = FontWeight.Bold)
                    }
                    Column(Modifier.weight(1f).padding(horizontal = 10.dp)) {
                        Text(
                            seat?.let { localName(res, it.name) + if (it.you) " (${res.getString(R.string.room_you)})" else "" }
                                ?: stringResource(R.string.room_seat_free),
                            color = if (seat == null) Tokens.faint else Tokens.text,
                        )
                        val asked = room.you.swapAsked.any { it.to == port }
                        if (asked) Text(stringResource(R.string.room_swap_waiting, port), color = Tokens.muted, fontSize = 12.sp)
                    }
                    if (seat != null && !seat.you && voices) {
                        TextButton(onClick = { session.toggleSilence(seat.name) }) {
                            Text(stringResource(if (seat.name in sound.silenced) R.string.room_unsilence else R.string.room_silence), color = Tokens.voice)
                        }
                    }
                    if (myPort != null && seat?.you != true) {
                        TextButton(onClick = { session.client.swapSeat(myPort, port) }) {
                            Text(
                                if (seat == null) stringResource(R.string.room_move_here, port) else stringResource(R.string.room_ask_swap, port),
                                color = Tokens.accent,
                            )
                        }
                    }
                }
            }
        }
        if (room.me is Me.Spectator && (room.you.hand || room.you.held)) {
            // Moved to watching by the host: back to playing through a raised hand.
            item { Notice(stringResource(if (room.you.hand) R.string.hand_up else R.string.hand_ask_host), modifier = Modifier.testTag("hand-note")) }
        }
        item {
            Row(Modifier.fillMaxWidth().padding(top = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                when {
                    room.me is Me.Spectator && (room.you.held || room.you.hand) -> {
                        val up = room.you.hand
                        val noHost = !up && !room.hostOnline
                        PrimaryButton(
                            stringResource(if (up) R.string.hand_lower else R.string.hand_raise),
                            { session.client.raiseHand(!up) },
                            Modifier.testTag("players-hand"),
                            enabled = !noHost,
                            icon = Icons.Filled.PanTool,
                        )
                    }
                    room.me is Me.Spectator -> PrimaryButton(stringResource(R.string.room_join_queue), { session.client.joinQueue() })
                    else -> SecondaryButton(stringResource(R.string.room_spectate), { session.client.spectate() })
                }
                // Volumes, microphone and output devices, and a test chime (Game settings).
                SecondaryButton(stringResource(R.string.sound_title), onSound, Modifier.testTag("players-sound"), icon = Icons.Filled.Tune)
            }
        }
        if (room.queue.isNotEmpty()) {
            item { Text(stringResource(R.string.room_queue_title), color = Tokens.muted, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 8.dp)) }
            items(room.queue) { q ->
                Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        "${ordinal(res, q.position)}  ${localName(res, q.name)}" + if (q.you) " (${res.getString(R.string.room_you)})" else "",
                        color = if (q.you) Tokens.accent else Tokens.text2,
                        modifier = Modifier.weight(1f),
                    )
                    // The queue talks too: silence by person.
                    if (!q.you && voices && q.name.isNotEmpty()) {
                        TextButton(onClick = { session.toggleSilence(q.name) }) {
                            Text(stringResource(if (q.name in sound.silenced) R.string.room_unsilence else R.string.room_silence), color = Tokens.voice)
                        }
                    }
                }
            }
        }
        if (room.spectators.isNotEmpty()) {
            item { Text(stringResource(R.string.room_spectators_title), color = Tokens.muted, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 8.dp)) }
            items(room.spectators) { s ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(localName(res, s.name) + if (s.you) " (${res.getString(R.string.room_you)})" else "", color = if (s.you) Tokens.accent else Tokens.text2)
                    if (s.hand) {
                        Spacer(Modifier.width(6.dp))
                        Icon(
                            Icons.Filled.PanTool,
                            contentDescription = stringResource(R.string.hand_raised, localName(res, s.name)),
                            tint = Tokens.accent,
                            modifier = Modifier.size(16.dp),
                        )
                    }
                }
            }
        }
    }
}

/** The pending pause request, over the picture: the host decides. */
@Composable
internal fun PauseAskedBanner(onCancel: () -> Unit, modifier: Modifier) {
    Surface(
        modifier.testTag("pause-banner"),
        color = Tokens.surface2,
        shape = RoundedCornerShape(50),
        border = BorderStroke(1.dp, Tokens.borderStrong),
        shadowElevation = 8.dp,
    ) {
        Row(Modifier.padding(start = 16.dp, end = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(10.dp).background(Tokens.accent, CircleShape))
            Spacer(Modifier.width(10.dp))
            Text(stringResource(R.string.room_pause_asked), color = Tokens.text, fontSize = 14.sp, modifier = Modifier.weight(1f, fill = false))
            TextButton(onClick = onCancel, modifier = Modifier.testTag("pause-cancel")) {
                Text(stringResource(R.string.room_pause_cancel_short), color = Tokens.accent, fontWeight = FontWeight.SemiBold)
            }
        }
    }
}

/** "<controller> · display only", over the picture while the pad only shows a real controller. */
@Composable
internal fun ControllerChip(name: String) {
    // A round, see-through button like the stats one; a tap shows the controller's name for a few seconds.
    var open by remember { mutableStateOf(false) }
    LaunchedEffect(open) {
        if (open) {
            kotlinx.coroutines.delay(4000)
            open = false
        }
    }
    val label = stringResource(R.string.room_pad_display_only_named, name)
    Row(
        Modifier
            .heightIn(min = 36.dp)
            .widthIn(max = 300.dp)
            .background(Color(0x8C05060A), RoundedCornerShape(50))
            .border(2.dp, Tokens.accent.copy(alpha = 0.6f), RoundedCornerShape(50))
            .clip(RoundedCornerShape(50))
            .clickable { open = !open }
            .semantics { contentDescription = label }
            .testTag("pad-display-only"),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (open) {
            Text(name, color = Tokens.text, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 12.dp).weight(1f, fill = false))
        }
        Box(Modifier.size(36.dp), contentAlignment = Alignment.Center) {
            Icon(Icons.Filled.Gamepad, contentDescription = null, tint = Tokens.accent, modifier = Modifier.size(18.dp))
        }
    }
}

/**
 * The stats button (top left over the picture) and, while on, a compact
 * see-through box next to it: frames per second decoded, the picture's
 * size and codec, the round trip to the device and the path (direct or
 * through the relay), packet loss, the game sound's codec, and the
 * screen's measured refresh rate (fps stays the game video's frame rate),
 * and what the device sends (stream_stats video: "768×448 (2× of
 * 384×224)" and the quality, like the website's stream figures).
 */
@Composable
internal fun StatsCorner(
    on: Boolean,
    stats: org.golink.player.core.LiveStatsView?,
    screenHz: Int?,
    onToggle: () -> Unit,
    modifier: Modifier,
    video: org.golink.player.core.StreamVideo? = null,
) {
    val cyan = Tokens.voice
    Row(modifier, verticalAlignment = Alignment.Top) {
        Box(
            Modifier
                .size(36.dp)
                .background(Color(0x8C05060A), CircleShape)
                .border(2.dp, if (on) cyan else cyan.copy(alpha = 0.6f), CircleShape)
                .clip(CircleShape)
                .toggleable(value = on, role = androidx.compose.ui.semantics.Role.Switch, onValueChange = { onToggle() })
                .testTag("room-stats"),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Filled.BarChart, contentDescription = stringResource(R.string.room_stats), tint = cyan, modifier = Modifier.size(18.dp))
        }
        if (on) {
            Spacer(Modifier.width(8.dp))
            val s = stats ?: org.golink.player.core.LiveStatsView()
            val dash = "–"
            val size = if (s.width != null && s.height != null) "${s.width}×${s.height}" else dash
            val path = when (s.path) {
                org.golink.player.core.NetPath.DIRECT -> stringResource(R.string.stats_direct)
                org.golink.player.core.NetPath.RELAY -> stringResource(R.string.stats_relay)
                org.golink.player.core.NetPath.UNKNOWN -> dash
            }
            val loss = s.lossPercent?.let { org.golink.player.core.LiveStatsMeter.formatLoss(it) } ?: dash
            val audio = if (s.audioCodec != null) listOfNotNull(s.audioCodec, s.audioKhz?.let { "$it kHz" }).joinToString(" ") else dash
            Surface(
                color = Color(0x9E05060A),
                shape = RoundedCornerShape(12.dp),
                border = BorderStroke(1.dp, cyan.copy(alpha = 0.35f)),
                modifier = Modifier.testTag("room-stats-box"),
            ) {
                Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
                    val mono = TextStyle(fontFamily = FontFamily.Monospace, fontSize = 12.sp, color = Tokens.text, lineHeight = 18.sp)
                    Text(stringResource(R.string.stats_line_video, s.fps?.toString() ?: dash, size, s.codec ?: dash), style = mono)
                    Text(stringResource(R.string.stats_line_net, s.rttMs?.toString() ?: dash, path), style = mono)
                    Text(stringResource(R.string.stats_line_loss, loss, audio), style = mono)
                    Text(stringResource(R.string.stats_line_screen, org.golink.player.core.ScreenRate.label(screenHz)), style = mono, modifier = Modifier.testTag("room-stats-screen"))
                    if (video != null) {
                        Text(stringResource(R.string.stats_line_size, videoSizeText(video)), style = mono, modifier = Modifier.testTag("room-stats-video"))
                        video.quality?.let { q -> Text(stringResource(R.string.stats_line_quality, videoQualityText(q, video.fallback)), style = mono) }
                    }
                }
            }
        }
    }
}

/** "768×448 (2× of 384×224)", or "384×224" at the game's own size (the website's videoRows). */
@Composable
internal fun videoSizeText(v: org.golink.player.core.StreamVideo): String =
    if (v.scale == 2) stringResource(R.string.stats_size_scaled, v.width * 2, v.height * 2, v.width, v.height) else "${v.width}×${v.height}"

/** "High", or "Saver (CPU)" when the room could not keep up with 2x. */
@Composable
internal fun videoQualityText(q: org.golink.player.core.VideoQuality, fallback: String?): String {
    val name = stringResource(
        when (q) {
            org.golink.player.core.VideoQuality.HIGH -> R.string.video_quality_high
            org.golink.player.core.VideoQuality.NORMAL -> R.string.video_quality_normal
            org.golink.player.core.VideoQuality.SAVER -> R.string.video_quality_saver
        },
    )
    return if (fallback == "cpu") stringResource(R.string.video_quality_cpu, name) else name
}
