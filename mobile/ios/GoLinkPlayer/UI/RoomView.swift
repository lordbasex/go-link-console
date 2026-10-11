// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import GoLinkCore
import SwiftUI

/**
 * The room: the game's picture with the on-screen gamepad around it, like
 * the website's console mode and the Android app. Portrait is a Game Boy
 * (picture on top, pad below); landscape is a Switch (pad halves on both
 * sides). Chat, players and your own settings live in the drawer (Chat /
 * Players / You); the Sound sheet opens from You.
 */
struct RoomView: View {
    @ObservedObject var session: RoomSession
    @EnvironmentObject private var model: AppModel
    @StateObject private var pad: TouchPadState
    @State private var touchOn: Bool
    @State private var drawer: DrawerTab?
    @State private var chatSeen = 0
    @State private var settingsOpen = false
    @State private var helpOpen = false
    @State private var micAsk = false
    @State private var lostText: String?
    @State private var statsOn: Bool
    /** With a real controller connected: the on-screen pad shown see-through (display only). */
    @State private var ghostPad = false
    /** 120 Hz on ProMotion screens while the room is open, and the measured rate for the stats. */
    @StateObject private var screenRate = ScreenRateMonitor()
    /** How this phone draws the game (Game settings › Picture). */
    @StateObject private var picture = PictureModel()
    /** The gamepad's skin (Game settings › Skin); nil is the classic pad. */
    @StateObject private var skins = SkinStore()

    init(session: RoomSession) {
        self.session = session
        _pad = StateObject(wrappedValue: TouchPadState { [weak session] bits in
            E2eProbe.touch(bits)
            session?.setTouchButtons(bits)
        })
        _touchOn = State(initialValue: Prefs().touchPad)
        _statsOn = State(initialValue: Prefs().stats)
    }

    private var ui: RoomUi { session.ui }
    private var room: RoomStateView? { ui.room }
    private var controls: GameControls { room?.controls ?? .default }
    private var streaming: Bool { ui.phase == .streaming }
    private var myPorts: [Int] { room?.myPorts ?? [] }
    private var starts: Int { startButtonCount(gamePlayers: controls.players, seated: room?.seated ?? 1) }
    private var aspect: CGFloat { CGFloat(ui.stats.aspect ?? 4.0 / 3.0) }
    private var hasController: Bool { !session.connectedControllers.isEmpty }
    // With a controller connected, the on-screen pad steps aside; the
    // gamepad button can bring it back see-through, showing the controller.
    private var showPad: Bool { streaming && (hasController ? ghostPad : touchOn) }
    private var admitted: Bool { ui.phase == .connecting || ui.phase == .streaming }
    private var userLines: Int { ui.chat.filter { if case .user = $0 { return true } else { return false } }.count }
    private var unread: Int { drawer == .chat ? 0 : max(0, userLines - chatSeen) }

    var body: some View {
        GeometryReader { g in
            let landscape = g.size.width > g.size.height
            ZStack {
                Tokens.bg.ignoresSafeArea()
                if showPad, let skin = skins.selected {
                    SkinConsoleLayout(
                        skin: skin,
                        picture: skins.background(skin, landscape: landscape),
                        landscape: landscape,
                        size: g.size,
                        insets: g.safeAreaInsets,
                        pad: pad,
                        controls: controls,
                        starts: starts,
                        myPorts: myPorts,
                        aspect: aspect,
                        keepDock: drawer != nil || settingsOpen,
                        header: { AnyView(header(compact: $0)) },
                        screen: AnyView(screen),
                        dock: { vertical in
                            AnyView(Group {
                                if vertical { VStack(spacing: 6) { dockItems() } } else { HStack(spacing: 6) { dockItems() } }
                            })
                        }
                    )
                    // The shell covers the whole screen (safe areas included), but the room's
                    // own size stays the safe area: otherwise this stack grew with it and the
                    // drawer, the settings and the name step sat below the screen's bottom
                    // (the chat's text field was off screen on an iPhone 17 Pro Max).
                    .frame(width: g.size.width, height: g.size.height, alignment: .topLeading)
                } else {
                    ConsoleLayout(
                        landscape: landscape,
                        size: g.size,
                        pad: pad,
                        controls: controls,
                        starts: starts,
                        myPorts: myPorts,
                        showPad: showPad,
                        aspect: aspect,
                        header: { AnyView(header(compact: $0)) },
                        screen: AnyView(screen),
                        dock: { AnyView(dock(vertical: $0)) },
                        noPad: AnyView(ChatPanel(session: session)),
                        floatingDock: AnyView(VStack(spacing: 6) { dockItems() }),
                        keepDock: drawer != nil || settingsOpen
                    )
                }
                if let drawer {
                    RoomDrawer(
                        session: session,
                        tab: Binding(get: { drawer }, set: { self.drawer = $0 }),
                        landscape: landscape,
                        unread: unread,
                        onSettings: {
                            self.drawer = nil
                            settingsOpen = true
                        },
                        onHelp: { helpOpen = true },
                        onLeave: { model.leaveRoom() },
                        onClose: { self.drawer = nil }
                    )
                    .transition(.move(edge: landscape ? .trailing : .bottom))
                }
                if settingsOpen {
                    GameSettingsPanel(
                        landscape: landscape,
                        picture: picture,
                        skins: skins,
                        sound: GameSound(
                            game: Binding(get: { session.sound.gameVolume }, set: { session.setGameVolume($0) }),
                            voices: Binding(get: { session.sound.voiceVolume }, set: { session.setVoiceVolume($0) }),
                            router: session.router,
                            test: { session.testSound() }
                        ),
                        initialName: model.prefs.playerName,
                        onSaveName: { model.setName($0) },
                        statsOn: Binding(get: { statsOn }, set: {
                            statsOn = $0
                            model.prefs.stats = $0
                        }),
                        onClose: { settingsOpen = false }
                    )
                    .transition(.move(edge: landscape ? .trailing : .bottom))
                    .zIndex(1)
                }
                if session.askName && admitted {
                    AliasView(initial: model.prefs.playerName, onEnter: { session.confirmName($0) }, onBack: { model.leaveRoom() })
                        .transition(.opacity)
                        .zIndex(2)
                }
                if let lostText {
                    Text(lostText)
                        .font(.subheadline).foregroundStyle(Tokens.text)
                        .padding(.horizontal, 16).padding(.vertical, 10)
                        .background(Capsule().fill(Tokens.surface2))
                        .frame(maxHeight: .infinity, alignment: .bottom)
                        .padding(.bottom, 24)
                        .onTapGesture { self.lostText = nil }
                }
            }
            .animation(.easeOut(duration: 0.2), value: drawer)
            .animation(.easeOut(duration: 0.2), value: settingsOpen)
            // A rotation swaps the whole layout: let go of every held
            // button (the device gets 0) before the new pad takes fingers.
            .onChange(of: landscape) { _, _ in pad.releaseAll() }
        }
        .ignoresSafeArea(.keyboard)
        .onChange(of: controls.control, initial: true) { _, c in pad.fourWay = TouchPadLogic.isFourWay(c) }
        .onChange(of: showPad) { _, _ in pad.releaseAll() }
        .onChange(of: hasController, initial: true) { _, has in pad.displayOnly = has }
        .onChange(of: session.controllerBits) { _, bits in pad.shown = controllerDisplayBits(bits, myPort: myPorts.first) }
        .onChange(of: statsOn, initial: true) { _, on in session.setStatsOn(on) }
        .onChange(of: room?.picture, initial: true) { _, p in picture.setRoomDefault(p) }
        .onChange(of: ui.chat.last) { _, line in
            // A declined pause request (or a raised hand's "not now") comes as a notice only to this guest.
            if case .system(_, _, .pauseDeclined?, _)? = line { toast(L("ev_pause_declined")) }
            if case .system(_, _, .handLater?, _)? = line { toast(L("hand_later")) }
        }
        .onChange(of: ui.refused) { _, r in
            // A refused raised hand or queue: say why for a moment.
            guard let r, let text = refusalText(r.code, handAgain: room?.you.handAgain ?? 0) else { return }
            toast(text)
        }
        .onChange(of: drawer) { _, d in if d == .chat || d == nil { chatSeen = userLines } }
        .onChange(of: userLines) { _, n in if drawer == .chat { chatSeen = n } }
        .onChange(of: session.router.lost) { _, lost in
            guard let lost else { return }
            toast(L("sound_input_lost", deviceName(lost.kind, lost.name)))
            session.router.lost = nil
        }
        .onAppear { screenRate.start() }
        .onDisappear {
            pad.releaseAll()
            screenRate.stop()
        }
        .sheet(isPresented: $helpOpen) { HelpSheet() }
        .alert(L("perm_mic_title"), isPresented: $micAsk) {
            if AudioRouter.micPermission == .denied {
                SwiftUI.Button(L("perm_open_settings")) {
                    if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) }
                }
            } else {
                SwiftUI.Button(L("perm_allow")) {
                    AudioRouter.requestMic { ok in if ok { session.setMic(true) } }
                }
            }
            SwiftUI.Button(L("perm_not_now"), role: .cancel) {}
        } message: {
            Text(AudioRouter.micPermission == .denied ? L("perm_mic_denied") : L("perm_mic_text"))
        }
    }

    /** A short message at the bottom of the screen for 4 seconds. */
    private func toast(_ text: String) {
        lostText = text
        DispatchQueue.main.asyncAfter(deadline: .now() + 4) { if lostText == text { lostText = nil } }
    }

    /**
     * The picture with the room's overlays. The renderer fills the whole
     * area and keeps the game's aspect inside it, so the Ambient and Frame
     * sides have room around the picture.
     */
    private var screen: some View {
        GeometryReader { g in
            ZStack {
                Tokens.video
                PictureView(feed: session.video, aspect: Double(aspect), native: ui.stats.video?.native, picture: picture)
                if streaming && picture.compare && picture.available {
                    CompareDivider(split: $picture.split, after: picture.settings.style.title)
                }
                RoomOverlay(session: session, onLeave: { model.leaveRoom() })
                if streaming, let room {
                    if room.recording {
                        RecBadge().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topTrailing).padding(8)
                    }
                    if let c = session.connectedControllers.first {
                        // Top right, round and see-through like the stats button (under the REC badge while recording).
                        ControllerChip(name: c.name)
                            .frame(maxWidth: min(300, g.size.width - 16), alignment: .trailing)
                            .padding(.top, room.recording ? 36 : 0)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topTrailing).padding(4)
                            .modifier(TrailingInsetPadding())
                    }
                    StatsCorner(on: statsOn, stats: session.liveStats, video: ui.stats.video, screenHz: screenRate.hz) {
                        statsOn.toggle()
                        model.prefs.stats = statsOn
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading).padding(4)
                    if room.you.pauseAsked != nil && !room.paused {
                        PauseBanner { session.client.cancelPauseRequest() }
                            .padding(.horizontal, 12)
                            .padding(.top, statsOn ? (ui.stats.video == nil ? 92 : (ui.stats.video?.quality == nil ? 108 : 124)) : 8)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                    }
                    if room.paused {
                        Text(L("room_paused_by", localName(room.pausedBy)))
                            .foregroundStyle(Tokens.text).padding(16)
                            .background(RoundedRectangle(cornerRadius: 12).fill(Tokens.video.opacity(0.8)))
                    }
                    VStack(spacing: 8) {
                        if session.voiceAsk && !session.askName {
                            VoiceAskCard(onYes: {
                                session.dismissVoiceAsk()
                                turnMicOn()
                            }, onNo: { session.dismissVoiceAsk() })
                        }
                        SwapOffers(room: room, client: session.client)
                    }
                    .frame(maxHeight: .infinity, alignment: .bottom).padding(8)
                }
            }
        }
        .clipped()
    }

    private func header(compact: Bool) -> some View {
        HStack(spacing: 4) {
            SwiftUI.Button { model.leaveRoom() } label: {
                Image(systemName: "xmark").font(.system(size: 17, weight: .semibold))
                    .frame(width: Tokens.control, height: Tokens.control)
            }
            .foregroundStyle(Tokens.text2)
            .accessibilityLabel(L("room_leave"))
            .accessibilityIdentifier("room-leave")
            if !compact {
                VStack(alignment: .leading, spacing: 1) {
                    let info = room?.info
                    Text(info.flatMap { !$0.title.isEmpty ? $0.title : (!$0.game.isEmpty ? $0.game : nil) } ?? L("app_name"))
                        .font(.system(size: 16, weight: .semibold)).foregroundStyle(Tokens.text).lineLimit(1)
                    if let me = room?.me {
                        Text(meLine(me)).font(.caption).foregroundStyle(Tokens.muted).lineLimit(1)
                            .accessibilityIdentifier("room-seat")
                    } else if let host = info?.host, !host.isEmpty {
                        Text(L("room_host", host)).font(.caption).foregroundStyle(Tokens.muted).lineLimit(1)
                    }
                }
                Spacer()
            }
        }
        .padding(.horizontal, compact ? 0 : 4)
    }

    // MARK: Dock

    private func dock(vertical: Bool) -> some View {
        Group {
            if vertical {
                VStack(spacing: 6) { dockItems() }.frame(maxHeight: .infinity)
            } else {
                HStack(spacing: 6) { dockItems() }.frame(maxWidth: .infinity).padding(.vertical, 6)
            }
        }
    }

    /** Turns the microphone on, asking for it first when the system has not allowed it yet. */
    private func turnMicOn() {
        if AudioRouter.micPermission == .granted {
            session.setMic(true)
        } else {
            micAsk = true
        }
    }

    /** The room's menu: microphone, a spectator's raised hand, sound, pause, chat, players, the pad and Game settings. */
    @ViewBuilder private func dockItems() -> some View {
        let seated = room?.me.isPlayer ?? false
        // The players and the queue talk; spectators listen.
        let talker = room?.talker ?? false
        let voiceOn = room?.voice != false
        let canTalk = talker && voiceOn
        let sound = session.sound
        DockButton(
            icon: sound.micOn && canTalk ? "mic.fill" : "mic.slash",
            label: L(!voiceOn ? "room_voice_off" : !talker ? "room_voice_players_only" : AudioRouter.micPermission == .denied ? "room_mic_blocked" : sound.micOn ? "room_mic_on" : "room_mic_off"),
            on: sound.micOn && canTalk,
            enabled: canTalk,
            tag: "dock-mic"
        ) {
            if sound.micOn {
                session.setMic(false)
            } else {
                turnMicOn()
            }
        }
        if streaming, let room, room.me == .spectator { handButton(room) }
        DockButton(
            icon: session.gameMuted ? "speaker.slash.fill" : (session.router.output == .speaker ? "speaker.wave.2.fill" : "headphones"),
            label: L(session.gameMuted ? "room_sound_off" : "room_sound_on"),
            on: !session.gameMuted,
            tag: "dock-sound"
        ) { session.setGameMuted(!session.gameMuted) }
        if let room, seated { pauseButton(room) }
        DockButton(icon: "bubble.left.and.bubble.right", label: L("room_chat"), on: drawer == .chat, badge: unread, tag: "dock-chat") {
            drawer = drawer == .chat ? nil : .chat
        }
        DockButton(icon: "person.2", label: L("room_players"), on: drawer == .players, tag: "dock-players") {
            drawer = drawer == .players ? nil : .players
        }
        if hasController {
            // The pad only shows what the controller presses.
            DockButton(icon: "gamecontroller", label: L(ghostPad ? "pad_display_hide" : "pad_display_show"), on: ghostPad, tag: "dock-pad") {
                ghostPad.toggle()
            }
        } else {
            DockButton(icon: "gamecontroller", label: L("room_touchpad"), on: touchOn, tag: "dock-pad") {
                touchOn.toggle()
                model.prefs.touchPad = touchOn
                if !touchOn { pad.releaseAll() }
            }
        }
        DockButton(icon: "gearshape", label: L("room_settings"), on: settingsOpen, tag: "dock-settings") {
            drawer = nil
            settingsOpen.toggle()
        }
    }
}

extension RoomView {
    /**
     * A spectator raises a hand to ask the host to play (the device allows
     * one raise a minute, and only while the host is in the room).
     */
    fileprivate func handButton(_ room: RoomStateView) -> some View {
        let up = room.you.hand
        let away = !up && !room.hostOnline
        return DockButton(
            icon: up ? "hand.raised.fill" : "hand.raised",
            label: L(up ? "hand_lower" : "hand_raise") + (away ? ". " + L("hand_no_host") : ""),
            on: up,
            dimmed: away,
            tag: "dock-hand"
        ) {
            if away {
                toast(L("hand_no_host"))
            } else {
                session.client.raiseHand(!up)
                if !up { toast(L("hand_up")) }
            }
        }
    }

    /**
     * Only the host pauses and resumes. A player asks for a pause (and can
     * withdraw the request); the button explains why it cannot when the
     * host is away or the game cannot be paused.
     */
    fileprivate func pauseButton(_ room: RoomStateView) -> some View {
        if room.paused {
            return DockButton(icon: "pause.fill", label: L("pause_by_host"), on: true, dimmed: true, tag: "dock-pause") {
                toast(L("pause_by_host_note"))
            }
        }
        if room.you.pauseAsked != nil {
            return DockButton(icon: "xmark", label: L("pause_cancel_request"), on: true, tag: "dock-pause") {
                session.client.cancelPauseRequest()
            }
        }
        if room.pausable && room.hostOnline {
            return DockButton(icon: "pause.fill", label: L("pause_ask"), on: false, tag: "dock-pause") {
                session.client.requestPause()
                toast(L("pause_asked_banner"))
            }
        }
        let why = L(room.pausable ? "pause_host_away" : "pause_not_pausable")
        return DockButton(icon: "pause.fill", label: L("pause_ask") + ". " + why, on: false, dimmed: true, tag: "dock-pause") { toast(why) }
    }
}

extension Me {
    var isPlayer: Bool {
        if case .player = self { return true }
        return false
    }
}

/** A skin's menu colors for the dock buttons inside it; nil keeps the app's own. */
private struct DockStyleKey: EnvironmentKey {
    static let defaultValue: PadSkin.MenuStyle? = nil
}

extension EnvironmentValues {
    var dockStyle: PadSkin.MenuStyle? {
        get { self[DockStyleKey.self] }
        set { self[DockStyleKey.self] = newValue }
    }
}

struct DockButton: View {
    @Environment(\.dockStyle) private var style
    let icon: String
    let label: String
    let on: Bool
    var enabled = true
    /** Drawn like a disabled button but still tappable, to explain why. */
    var dimmed = false
    var badge = 0
    let tag: String
    let action: () -> Void

    var body: some View {
        SwiftUI.Button(action: action) {
            let accent = style.map { Color(hex: $0.active) } ?? Tokens.accent
            let icon2 = style.map { Color(hex: $0.icon) } ?? Tokens.text2
            let fill = style.map { Color(hex: $0.button) } ?? Tokens.surface
            let activeFill = style.map { Color(hex: $0.activeButton) } ?? Tokens.accentTint
            Image(systemName: icon)
                .font(.system(size: 17, weight: .medium))
                .foregroundStyle(!enabled || dimmed ? Tokens.dim : (on ? accent : icon2))
                .frame(width: Tokens.control, height: Tokens.control)
                .background(Circle().fill(!enabled || dimmed ? (style == nil ? Tokens.sunken : fill.opacity(0.5)) : (on ? activeFill : fill)))
                .overlay(Circle().stroke(on ? (style == nil ? Tokens.accentTintBorder : accent.opacity(0.5)) : (style == nil ? Tokens.border : icon2.opacity(0.18)), lineWidth: 1))
                .overlay(alignment: .topTrailing) {
                    if badge > 0 {
                        Text("\(min(badge, 99))")
                            .font(.system(size: 11, weight: .bold)).foregroundStyle(Tokens.onAccent)
                            .frame(minWidth: 18, minHeight: 18)
                            .background(Capsule().fill(Tokens.accent))
                            .offset(x: 4, y: -4)
                            .accessibilityIdentifier("\(tag)-badge")
                    }
                }
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
        .accessibilityLabel(label)
        .accessibilityIdentifier(tag)
    }
}

struct RecBadge: View {
    var body: some View {
        HStack(spacing: 6) {
            Circle().fill(Tokens.rec).frame(width: 8, height: 8)
            Text(L("room_rec")).font(.system(size: 12, weight: .bold)).foregroundStyle(Tokens.text)
        }
        .padding(.horizontal, 10).padding(.vertical, 4)
        .background(Capsule().fill(Tokens.video.opacity(0.8)))
        .overlay(Capsule().stroke(Tokens.rec, lineWidth: 1))
    }
}

/** Joining, the PIN, connecting, or why the room is not available. */
struct RoomOverlay: View {
    @ObservedObject var session: RoomSession
    let onLeave: () -> Void
    @State private var pin = ""

    var body: some View {
        let ui = session.ui
        if ui.knock == .waiting {
            knockCard(title: L("knock_waiting_title"), text: L("knock_waiting_text"), hand: true, button: L("knock_leave"))
        } else if case let .ended(reason) = ui.knock {
            knockCard(title: L("knock_ended_title"), text: knockEndedText(reason), hand: false, button: L("knock_back"))
        } else if let text = progressText(ui) {
            VStack(spacing: 12) {
                ProgressView().tint(Tokens.accent).controlSize(.large)
                Text(text).foregroundStyle(Tokens.text).multilineTextAlignment(.center)
                    .accessibilityIdentifier("room-progress")
            }
            .padding(20)
            .background(RoundedRectangle(cornerRadius: 16).fill(Tokens.video.opacity(0.8)))
        } else if ui.phase == .pin && ui.pin.needed && !ui.pin.busy {
            pinPrompt(ui)
        } else if let problem = problemKey(ui.phase) {
            VStack(alignment: .leading, spacing: 12) {
                Text(L(problem)).foregroundStyle(Tokens.text).accessibilityIdentifier("room-problem")
                if ui.phase == .removed {
                    Text(L("hand_removed_text")).font(.subheadline).foregroundStyle(Tokens.muted)
                }
                HStack(spacing: 8) {
                    if ui.phase == .rateLimited || ui.phase == .unreachable {
                        PrimaryButton(title: L("room_retry")) { session.client.reconnect() }
                    }
                    SecondaryButton(title: L("room_leave"), action: onLeave)
                }
            }
            .padding(18)
            .frame(maxWidth: 420)
            .background(RoundedRectangle(cornerRadius: 16).fill(Tokens.surface))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(Tokens.border, lineWidth: 1))
            .padding(16)
        }
    }

    private func progressText(_ ui: RoomUi) -> String? {
        switch ui.phase {
        case .joining: return L(ui.reconnecting ? "room_reconnecting" : "room_joining")
        case .connecting: return L("room_connecting")
        case .pin: return ui.pin.needed && !ui.pin.busy ? nil : L("room_checking_pin")
        default: return nil
        }
    }

    private func problemKey(_ phase: RoomPhase) -> String? {
        switch phase {
        case .ended: return "room_ended"
        case .notFound: return "room_not_found"
        case .rateLimited: return "room_rate_limited"
        case .unreachable: return "room_unreachable"
        case .removed: return "hand_removed_title"
        default: return nil
        }
    }

    /**
     * A group invitation: waiting while the host decides (the website's
     * KnockPrompt), or why the way in closed.
     */
    private func knockCard(title: String, text: String, hand: Bool, button: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .top, spacing: 12) {
                if hand {
                    Text("✋").font(.system(size: 30)).accessibilityHidden(true)
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text(title).font(.headline).foregroundStyle(Tokens.text)
                        .accessibilityIdentifier("room-knock-title")
                    Text(text).font(.subheadline).foregroundStyle(Tokens.muted).fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("room-knock-text")
                }
            }
            SecondaryButton(title: button, action: onLeave).accessibilityIdentifier("room-knock-leave")
        }
        .padding(18)
        .frame(maxWidth: 420)
        .background(RoundedRectangle(cornerRadius: 16).fill(Tokens.surface))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(Tokens.border, lineWidth: 1))
        .padding(16)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(hand ? "room-knock-waiting" : "room-knock-ended")
    }

    /** The room asks again: the PIN the person typed failed (or a return token expired). */
    private func pinPrompt(_ ui: RoomUi) -> some View {
        let last = ui.pin.last
        let stop = last?.reason == "blocked" || last?.reason == "locked"
        return VStack(alignment: .leading, spacing: 10) {
            Text(L("pin_prompt_title")).font(.headline).foregroundStyle(Tokens.text)
            if let last {
                Notice(text: pinRefusal(last), danger: true).accessibilityIdentifier("room-pin-error")
            } else {
                Text(L("pin_prompt_text")).font(.subheadline).foregroundStyle(Tokens.muted)
            }
            if !stop {
                TextField("", text: $pin, prompt: Text("000000").foregroundStyle(Tokens.placeholder))
                    .keyboardType(.numberPad)
                    .kerning(6)
                    .onChange(of: pin) { _, v in
                        let d = String(v.filter(\.isASCIIDigit).prefix(6))
                        if d != v { pin = d }
                    }
                    .goLinkField(mono: true, big: true)
                    .accessibilityIdentifier("room-pin")
            }
            HStack(spacing: 8) {
                if !stop {
                    PrimaryButton(title: L("pin_send"), enabled: Invites.isPin(pin)) {
                        session.client.submitPin(pin)
                        pin = ""
                    }
                    .accessibilityIdentifier("room-pin-send")
                }
                SecondaryButton(title: L("room_leave"), action: onLeave)
            }
        }
        .padding(16)
        .frame(maxWidth: 380)
        .background(RoundedRectangle(cornerRadius: 16).fill(Tokens.surface))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(Tokens.border, lineWidth: 1))
        .padding(12)
    }
}

/** Requests to swap controllers waiting for this player's answer. */
struct SwapOffers: View {
    let room: RoomStateView
    let client: RoomClient

    var body: some View {
        if let offer = room.you.swapOffers.first {
            VStack(alignment: .leading, spacing: 8) {
                Text(L("room_swap_offer", localName(offer.name), offer.from, offer.to)).font(.subheadline).foregroundStyle(Tokens.text)
                HStack(spacing: 8) {
                    PrimaryButton(title: L("room_accept")) { client.answerSwap(from: offer.from, to: offer.to, accept: true) }
                        .accessibilityIdentifier("swap-accept")
                    SecondaryButton(title: L("room_decline")) { client.answerSwap(from: offer.from, to: offer.to, accept: false) }
                }
            }
            .padding(12)
            .frame(maxWidth: 420)
            .background(RoundedRectangle(cornerRadius: 14).fill(Tokens.surface))
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(Tokens.accentTintBorder, lineWidth: 1))
        }
    }
}

/**
 * "Do you want to join the voice?": a small card under the picture when
 * this app becomes able to talk with its microphone off (never with the
 * name step; it goes away by itself, see GoLinkCore.VoiceAsk).
 */
struct VoiceAskCard: View {
    let onYes: () -> Void
    let onNo: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(L("voice_ask_title"), systemImage: "mic.fill")
                .font(.subheadline).foregroundStyle(Tokens.text)
                .accessibilityIdentifier("voice-ask-title")
            HStack(spacing: 8) {
                PrimaryButton(title: L("voice_ask_yes"), action: onYes).accessibilityIdentifier("voice-ask-yes")
                SecondaryButton(title: L("voice_ask_no"), action: onNo).accessibilityIdentifier("voice-ask-no")
            }
        }
        .padding(12)
        .frame(maxWidth: 420)
        .background(RoundedRectangle(cornerRadius: 14).fill(Tokens.surface))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(Tokens.accentTintBorder, lineWidth: 1))
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("voice-ask")
    }
}
