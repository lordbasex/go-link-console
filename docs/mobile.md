# The Android app (go-link Console Player)

`mobile/android/` is **go-link Console Player**, a native Android app (Kotlin, Jetpack Compose) for joining a game as a player. It is not a web view: it speaks the same signaling and WebRTC protocol as the website ([protocol.md](protocol.md), signalhub's README) with a prebuilt libwebrtc. It only joins rooms. Hosting stays on the device app.

| | |
|---|---|
| Package | `org.golink.player` |
| Android | 8.0 (API 26) or newer; built against API 37, targets API 36 |
| Distribution | An APK attached to the GitHub releases (`go-link-console-vX.Y.Z-android.apk`), not in Play Store yet |
| Build, sign, install | [`mobile/android/README.md`](../mobile/android/README.md) |

## Layout

| Module | What it is |
|---|---|
| `:core` | Plain Kotlin, no Android types, tested on the JVM: the signalhub envelope and client (`SignalClient`, reconnect with jittered backoff, FIFO requests), invitation parsing, signaling URL rules, ICE servers from `hello`, the `input` packet (byte for byte the web's `encodeInput`), the touch pad and gamepad mapping, `room_state` / `chat` / `typing` / `stream_stats` parsing, the PIN gate, the Sound sheet's device choices (`AudioDevices.kt`), the player name rules (`PlayerName.kt`), the stats overlay's arithmetic and the input latency meter (`LiveStats.kt`), and `RoomClient`, which runs one room visit. It is the reference for an iOS (Swift) port. |
| `:app` | Android: Compose screens, OkHttp WebSockets, `AndroidRtcPeer` (stream-webrtc-android), the audio route, controllers, CameraX + ML Kit for the QR code, SharedPreferences. |

`RoomClient` is a port of the website's `useJoinRoom` + `useHostStream` + `HostStream`; the platform plugs in a WebSocket factory and an `RtcPeer` factory. Web files it mirrors: `packages/shared/src/{protocol,signal-client,stream,room-state,touch-pad,gamepad}.ts` and `apps/web/src/pages/roomModel.ts`.

## Joining

1. **Scan the QR code** (the invitation link `https://play.go-link.org/g/<invite>`), **type the 9-digit code** (or paste the link), or open an invitation link with the app installed (Android App Links).
2. The PIN screen: the **6-digit PIN** the host made for this person, and the terms checkbox (`Terms.VERSION` equals `TERMS_VERSION` in `frontend/apps/web/src/legal.ts`, a test checks it).
3. `join` with `invite` or `code` → `joined`; the device sends `pin_required` and the app sends the typed PIN; `pin_result` admits it (or says why not: wrong, used, blocked, locked). Then the device offers and the app answers.
4. **Your name** (the first admission of each visit, not after a reconnect): a "What's your name?" screen over the room, prefilled with the saved name, and **Enter the room**. See [Your name](#your-name).

Rules:

- **The PIN is never in a QR code or a link.** The QR code stays the website's invitation link.
- **Every new join asks for the PIN**, even when a return token for that room is stored. The `pin_result` token (kept per `room_id` in SharedPreferences `go-link.room-passes`, the last 20, like the website's localStorage) is only used to get back in by itself after the connection drops (network change, app in the background) during the same visit. A refused token is forgotten and the person is asked again.
- **A QR code is parsed strictly**: only exactly `https://play.go-link.org/g/<22 base64url characters>` (an optional trailing `/`) or a 9-digit code. Other hosts, `http://`, ports, user info, query strings and fragments are rejected. A QR code or a link never changes the signaling server or carries a PIN.
- The App Link intent filter only takes `https://play.go-link.org/g/…`, and only the path is read.

## Signaling server

The app uses `wss://signal.go-link.org/ws?v=1`. It sends no `Origin` header (signalhub accepts native clients) and `app` is `go-link`. STUN and TURN come only from `hello`, kept in memory and taken again on every reconnect. Nothing is hardcoded.

Settings has **Signaling server**, like the website's button: only `wss://`, tested for a `hello` before saving, a notice on the home screen while a custom server is active, and **Back to the official server**. Links and QR codes never change it. The network security config forbids clear-text traffic.

**Debug builds only** also accept `ws://` to the development machine (`localhost`, `127.0.0.1`, and `10.0.2.2`, the emulator's address for the computer), like the website's loopback exception: `SignalUrls.check(url, allowDevHosts = BuildConfig.DEBUG)` and a debug-only network security config (`app/src/debug/res/xml`). It is set by hand in Settings, like any server. Release builds keep `wss://` only and never take a server from a link or an intent.

## The room

- **Media:** VP8 video (drawn by an `EglRenderer` into a `SurfaceView` that covers the whole picture area, with our picture drawer keeping the display `aspect` from `stream_stats`, since arcade pixels are not square; see [Game settings and picture styles](#game-settings-and-picture-styles)), Opus game sound, and the `voice-p1`…`voice-p4` tracks. Per-player silence and a game sound switch use each remote track's volume.
- **Voice:** the microphone m-line the device offers `recvonly` is answered `sendonly` (`RtcSignals.micMid`), and the mic track is set with `RtpSender.setTrack`, so turning it on never renegotiates. Only seated players can talk in rooms with voice (the device enforces it too).
- **Channels:** `control` (JSON: `hello` with the name and local players, `chat`, `typing`, `spectate`, `queue`, `swap_seat`, `swap_answer`, `pause_request`, and `pong` to the device's pings) and `input` (12-byte packets, repeated every 100 ms while held, two extra releases). `files` is refused: it is only for the host's own browser.
- **Layout:** portrait is a Game Boy (picture on top, dock, gamepad below); landscape is a Switch (gamepad halves on both sides, dock on the right). Full screen, screen kept on.
- **On-screen gamepad:** buttons from `room_state.controls` (D-pad with 4-way games limited to four directions, 1 to 6 action buttons, Coin, 1P…NP start buttons from `startButtonCount`). Several fingers at once, a finger can slide between buttons, a short vibration on each press. Hidden while a physical controller is connected (the dock's gamepad button then shows it see-through, display only: see [Controllers](#controllers-see-through-pad-and-the-controller-test)), and with the gamepad button of the dock.
- **Controllers:** Android `KeyEvent` and `MotionEvent` from Bluetooth and USB pads, mapped to the browser's standard Gamepad layout (A = bottom = button 1, B = right, X = left, Y = top, L1/R1, L2/R2 as keys or triggers, Select = Coin, Start, sticks and hats; the left stick also presses the directions). Each controller is a local player in the order it is first used (the first one shares player 0 with the touch pad), and `hello.local_players` lists them (the device seats only the first one of a guest: one guest, one seat).
- **Chat and players:** a sheet with the chat (system notices translated from their `event`, generated guest names translated, the typing indicator) and the seats (move to a free seat, ask to swap, silence a voice), the queue, spectators, and Just watch / Join the queue. Swap requests show over the picture with Accept / Decline. Only the host pauses and resumes: seated players **ask for a pause** (see [Pause requests](#pause-requests)).
- **Sound:** part of **Game settings** (`ui/SoundSection.kt`; the **Sound** button in the players tab opens Game settings scrolled to it) with two lists of radio rows. **Microphone:** Automatic, Phone microphone, and each connected headset's microphone (Bluetooth, wired, USB). **Output:** Automatic, Speaker, and each connected headset (Bluetooth, wired, USB, hearing aid). The earpiece is never offered. **Test sound** plays a short chime in the voice-communication path, so it comes out where the game and the voices do. See [Sound devices](#sound-devices).

## Your name

The same rules as the device's `hello.name` (`PlayerName` in `:core` and GoLinkCore, with the same tests): after NFC normalization, trimming and collapsing runs of spaces, **2 to 20 characters** (code points), only Unicode letters (`\p{L}`, accents and ñ included), digits (`\p{N}`) and spaces. Symbols and emoji are refused.

- **The name step** (after the PIN, once per visit): the design's "What's your name?" screen. The field's border is grey, orange when valid and red when it has a symbol or emoji; the hint under it says why ("Only letters, numbers and spaces (no symbols or emoji).", "At least 2 characters.", "20 characters at most."), with an `n/20` counter. **Enter the room** is enabled only for a valid name, saves it for the next game and sends a new `hello`. Back leaves the room. The room connects behind the screen, so the first `hello` carries the saved name (or none, and the device picks a guest name) until Enter.
- **Changing it** in the room (drawer › You on iOS, Settings) uses the same field; empty is allowed there (the device then names you).
- Whatever is sent goes through `PlayerName.sanitize` (only the allowed characters, collapsed, cut to 20; empty when fewer than 2 are left). The device sanitizes again and reports the name it uses in `room_state.you.name`.

## Pause requests

Pause belongs to the host (the device's owner). The apps are always guests and never send `pause` (the device refuses it with `code: "pause_owner_only"`):

- The dock's pause button (`dock-pause`, seated players only) is **Ask for a pause** → `{"type":"pause_request"}`, with a notice "You asked the host for a pause…". While `room_state.you.pause_asked` is set, a banner over the picture (`pause-banner`) and the button offer **Cancel** → `{"type":"pause_request","cancel":true}`.
- It is dimmed, and a tap explains why, when the game is paused ("Paused by the host": only the host resumes), when the game is not pausable, or when `room_state.host_online` is false ("The host isn't in the room to answer."). Older devices without `host_online` count as online.
- Accepted: the usual `paused: true` with `paused_by`. Declined: the chat notice event `pause_declined` (sent only to the one who asked), shown translated in the chat and as a short notice.

## Stats overlay

A small round stats button (cyan ring, `room-stats`) at the top left of the picture toggles a compact, see-through box next to it (`room-stats-box`) that never covers the center:

```
60 fps · 768×448 VP8
ping 28 ms · direct
loss 0 % · audio Opus 48 kHz
screen 120 Hz
video 768×448 (2× of 384×224)
quality High
```

The last two lines come from the device's `stream_stats.video` (see [protocol.md › Video scale](protocol.md#video-scale)), like the website's stream figures: the frames sent and the game's own size ("384×224" alone at scale 1, for example the test pattern room's 640×480), and in game rooms the quality in use (High, Normal, Saver; "Saver (CPU)" after the device's automatic fallback). They are missing with older devices, which do not send `video`.

Read once a second from libwebrtc's statistics (Android `PeerConnection.getStats`, iOS `RTCPeerConnection.statistics`): the video `inbound-rtp` (`framesDecoded`, `frameWidth`/`frameHeight`, the codec's `mimeType`), the video and game audio packets received and lost, the game audio codec's `clockRate`, and the transport's selected candidate pair (or the nominated succeeded one): `currentRoundTripTime` and its candidate types (`relay` on either side = relay, otherwise direct). `LiveStatsMeter` turns the cumulative counters into the last second's frame rate and loss; unknown values show a dash. The choice is remembered (Android SharedPreferences `stats-overlay`, iOS UserDefaults `go-link.stats`; off by default).

`fps` is always the game video's decoded frame rate. The last line is the phone's **screen refresh rate**, measured, not the panel's nominal maximum: the median time between screen frames over a second (`RefreshRateMeter` in `ScreenRate.kt` / `ScreenRate.swift`, snapped to a common panel rate within 3 %), from Choreographer frames on Android and a `CADisplayLink` on iOS. Battery saver, Low Power Mode, heat or the phone's own smooth display setting can keep it lower, and the line then shows it.

## Game settings and picture styles

The dock's gear (**Settings**, `dock-settings`, right under the gamepad button) opens **Game settings** without leaving the room: a side sheet from the right in landscape, a bottom sheet in portrait, so the picture stays in sight while it changes. The drawer's You tab opens the same panel (its **Game settings** button replaced Volume and voice and the name field there). It holds:

- **Your name** with the name rules of the name step; Save sends a new `hello`.
- **Sound:** the game and voices volumes, 0 to 100 % (remembered; older builds allowed up to 300 %, which now reads as 100 %), the microphone and output choices and **Test sound** (what the Sound sheet had). The dock keeps its game sound switch.
- **Picture:** the style (**Smooth**, **Sharp**, **CRT arcade**, **Smooth edges**), the sides (**Black**, **Ambient**, **Frame**) and **Compare**: a line over the live picture, draggable (VoiceOver and TalkBack adjust it like a slider), with the plain decoded picture on the left ("Original": Smooth on Black) and the chosen style on the right. Kept per phone in `go-link.picture-style` / `go-link.picture-bands` (the website's keys and values), applied at once without reconnecting. Defaults: Smooth with Ambient sides (`PictureSettings.defaultStyle` / `defaultBands` in GoLinkCore, `PictureSettings.DEFAULT_STYLE` / `DEFAULT_BANDS` in `:core`).
- **Skin**: the console shell around the picture (Violet, Red, Green, Blue, Smoke, Orange, and any skin added to the app's Skins folder: the Files app on iOS, `Android/data/org.golink.player/files/Skins` on Android). A skin is a JSON file that places every part (the picture, the menu capsule, the D-pad, the action buttons, Coin and the starts, the room's name) separately in portrait and landscape, and colors the plastic, the controls and the menu; it may bring its own background pictures. The built-in skins put the menu under the picture in portrait (always shown) and over the picture in landscape (it folds into a handle after 3 s; a tap on the picture or the handle brings it back). Kept per phone in `go-link.pad-skin`; Smoke until the player chooses (the old "Classic" choice reads as Smoke). Format, placement rules and the editor: [skins/README.md](skins/README.md). iOS installs a skin by pasting its JSON too: Settings › Gamepad skins › **Install skin** (also under the skins in Game settings) checks it (`SkinCheck`: JSON mistakes with line and column, built-in ids refused, the layout on phones and a tablet, missing pictures), previews it with the real painter in both orientations, saves it as `Skins/skin-<id>.json` (`SkinInstaller`) and offers to use it; it shows as Custom, and installed skins can be deleted there or from their tile's menu. `testInstallSkinByPasting` pastes a broken and a valid skin, installs, chooses and deletes it. Android does the same from Settings › **Skins** › **Install skin** (and the button under the skins in Game settings): `SkinInstall` in `:core` checks the text (JSON mistakes with line and column, built-in ids refused, the layout on the reference phones and tablets in both orientations, missing pictures; `SkinInstallTest`), the preview draws it with `SkinConsole` on a 402 x 874 dp phone in either orientation, and `SkinStore.install` saves `Skins/skin-<id>.json` (replacing an installed skin with that id, wherever it was); custom skins are listed in Settings with a delete button, show as Custom in Game settings and are deleted there with a long press (always after asking). The e2e `installs a pasted skin, previews it, uses it and deletes it` (`npm run test:android:skins`) types a broken and a valid skin into the field. Touch zones cover only the controls, so the picture keeps its taps (the PIN prompt, swap offers). iOS: the pad lab takes `-labSkin <id>` (`-labButtons N`, `-labStarts N`) and `testSkinPad` presses every control of each built-in skin in both orientations. Android (`ui/SkinPad.kt`): the debug lab `--es lab skin --es skin <id>` (`--ei buttons`, `--ei starts`), and `npm run test:android:skins` (e2e) presses the same controls on an emulator.
- **Show stats:** the same setting as the stats button over the picture.

**Room default.** The host may give a room a default picture, sent in `room_state.picture` (`{style, bands}`; parsed in `:core` `RoomState.kt` and GoLinkCore `RoomMessages.parsePicture`, nil unless both values are known). As on the website, the viewer's own saved choice wins field by field, then the room's default, then the app's (`resolve`). Game settings shows "Room default: *style* · *sides*" and, when the viewer's own choice shows something else, **Use the room's default** (`picture-use-room-default`), which forgets the viewer's choice. The apps are always guests, so they never set a room's default.

**Renderer.** Both apps draw the game with the website's shaders (`frontend/apps/web/src/picture/shaders.ts`, see [web.md › Picture styles](web.md#picture-styles)), the same math and the same looks. The picture area is the whole screen box, and the picture inside it is the largest rectangle with the stream's display aspect (`fitRect`, never cropped; the Frame sides inset it by `frameInset` so the bezel always shows); the layout math and the settings parsing live in GoLinkCore `Picture.swift` and `:core` `Picture.kt`, with tests mirroring `picture.test.tsx`. Each new frame is first converted from YUV to an RGB texture at its own size (BT.601, limited range unless the buffer says full), then the ambient pass (a 32 × 24 texture blended toward each frame), the smooth edges pass (an integer enlargement, at most 8×) and the picture pass run as on the web. It draws only when a new frame, a new setting or a new size arrives.

**2x streams.** Game rooms send the picture enlarged 2x with nearest neighbour (the High and Normal [video qualities](device.md#video-quality)). Both cores parse `stream_stats.video` (GoLinkCore `StreamVideo.swift`, `:core` `StreamVideo.kt`: `scale`, the game's `width` and `height`, `quality`, `fallback`; absent or broken means scale 1) and the room session keeps the last valid one in `stats.video`. When the decoded frame is **exactly** twice `video.width` × `video.height` (`workingSize`, as in the website's `renderer.ts`; tested in both cores), the renderer first averages each 2 × 2 block into a texture of the game's own size, and the ambient light, the smooth edges pass and every style read that texture; any other frame (scale 1, or the old size for a moment after a quality change) is drawn as it is. Metal (`down_fragment`) reads the block's four texels exactly (`texture.read`); GLES (`DOWN_SHADER` in `Shaders.kt`) takes one bilinear sample at the block's center, which gives the same average but never mixes in a neighbouring block even when a GPU's coordinates land a hair off a texel center (the website's four-sample shader was off by one step on about 0.5 % of the pixels on the Android emulator). The plain fallback views (WebRTC's own view, and Android's plain drawer for Smooth on Black without Compare) are unchanged: the 2x picture scales like any other. Debug checks: iOS `-picture2xCheck` and Android `--es lab check2x` draw the test card in every style and side offscreen, at its own size and as a 2x nearest-neighbour stream, and require identical bytes (iOS 24 of 24, 12 combinations at two sizes; Android 22 of 22, without Smooth on Black, which the plain drawer shows), while the same 2x frames drawn raw must differ; `testPicture2xMatchesNative` runs the iOS one. The picture labs take iOS `-labUp2` (`-labRaw`) and Android `--ez up2 true` (`--ez raw true`) to stream the card as a 2x stream.

If the GPU path cannot start (no GPU, a shader that does not compile) the room shows WebRTC's plain view, fitted to the aspect, and the Picture section says styles are not available.

- **Android** (`app/.../picture/`): `PictureDrawer` is a `RendererCommon.GlDrawer` given to the room's `EglRenderer`, which draws into a `SurfaceView` covering the picture area (it replaced the `TextureView`, which froze the app under load on the emulator). `Shaders.kt` holds the website's GLSL ES 1.00 word for word. Hardware decoder frames (OES textures), RGB textures and I420 planes are first drawn into an RGBA texture at the frame's size, row 0 at the top like a WebGL upload. The ambient mix drops to 0.05 when the system's animations are removed. Smooth on Black without Compare uses libwebrtc's plain drawer (like the website's `needsRenderer`). A setting change redraws the last I420 frame on a worker thread; hardware decoder frames are never held (that stalls the decoder), so the next frame shows the change. The debug picture lab is `--es lab picture` (`--es style`, `--es bands`, `--ez compare`, `--ez sheet`, `--es room crt:frame`, `--ez plain true`, `--ez up2 true`, `--ez raw true`; `--es lab check2x` is the offscreen 2x check). The emulator draws with SwiftShader on the CPU, so its frame rates say nothing about a phone.
- **iOS** (`GoLinkPlayer/Picture/`): `PictureShaders.swift` is a line by line Metal Shading Language port, compiled at run time (`makeLibrary(source:)`, so building needs no Metal toolchain; keep it in sync with `shaders.ts`). `PictureMTKView` is an `MTKView` that is also WebRTC's `RTCVideoRenderer`: frames only replace the pending one; the view ticks at 120 Hz (`preferredFramesPerSecond`) and draws only when something changed, so a 60 fps game reaches the screen within one 8 ms tick. I420 buffers (VP8's software decoder) are uploaded into a ring of three plane textures, NV12 `CVPixelBuffer`s are used directly through a `CVMetalTextureCache`. `PictureRenderer` has no UIKit or WebRTC (it can render offline). `PictureView` picks it or the fallback (`-plainPicture` in debug builds forces the fallback).

## High refresh rate (90, 120, 144 Hz)

Only the room and the controller test ask for the screen's highest refresh rate; the menus stay at the system's choice, which saves battery.

- **iOS / iPadOS (ProMotion):** `CADisableMinimumFrameDurationOnPhone` is `YES` in the Info.plist (without it iPhones cap apps at 60 Hz). While the room or the test is open, a `CADisplayLink` asks for `CAFrameRateRange(minimum: 60, maximum: 120, preferred: 120)` (`UI/ScreenRateMonitor.swift`), which keeps ProMotion at 120 Hz and measures the rate. The game picture's own `MTKView` (`PictureMTKView`, see [Game settings and picture styles](#game-settings-and-picture-styles)) ticks 120 times a second and draws each new frame on the next tick, so it reaches the screen within about 8 ms instead of 17 (the fallback `RTCMTLVideoView` gets the same 120 Hz). At 120 Hz UIKit also delivers the pad's touches at 120 Hz.
- **Android:** the room and the test set the window's `preferredDisplayModeId` to the mode with the current resolution and the highest refresh rate (`ScreenRate.fastest`, `ui/HighRefreshRate.kt`) and put the previous choice back when they close. The window's mode also covers the game picture's `SurfaceView`.

## Controllers, see-through pad and the controller test

- **See-through pad:** with a physical controller connected, the on-screen pad stays hidden; the dock's gamepad button shows it at about 55 % opacity and **display only** (touches do nothing), lighting the directions and buttons pressed on the real controller with the same mapping that is sent to the device. Controllers connecting and disconnecting are followed live.
- **Controller button:** while a controller is connected, a round see-through button with the controller icon sits at the top right of the picture (like the stats button at the top left; under the REC badge while recording; clear of the floating capsule in cinema mode). A tap shows the controller's name for 4 s (`pad-display-only` on Android, `pad-controller-chip` on iOS).
- **Cinema mode:** in landscape without the on-screen pad (a controller in hand, or the pad turned off), the picture is as large as the screen allows, its sides filled by the picture's sides style (Ambient by default), and the leave button and the room's buttons ride in a floating capsule on the right that folds into a handle after 3 s (never while a sheet is open); a tap on the picture or the handle brings it back. The gamepad button brings the skin back with the see-through pad. Labs: iOS `-padLab -labNoPad` (`testCinemaLayout`), Android `--es lab cinema`.
- **Test controller** (home screen, `home-test-controller`): an offline screen (`test-controller-screen`), no network at all. A PM5544-style test card (grey grid, color bars, circle) with a controller diagram (D-pad, buttons 1 to 6, Coin, Start) that lights up for the on-screen pad and for real controllers; the connected controller's name and connection, and the **input latency**: from the input event's time to the next frame drawn after it (Android `withFrameNanos`, iOS `CADisplayLink`), last, minimum and average of the last 60 (`LatencyMeter`), and the measured **screen refresh rate** with one frame's time ("screen 120 Hz · 1 frame 8.3 ms"): the latency is about one screen frame (8 ms at 120 Hz, 17 ms at 60 Hz), which the hint explains. The screen runs at its highest rate while the test is open. It wears the player's gamepad skin (Smoke until they pick one, the same `SkinConsoleLayout`/`SkinConsole` as a room): the test card is the game in the skin's bezel with the readouts over its lower part, the skin's D-pad, buttons 1 to 6, Coin and a Start pill (the player's own Start, lit by a controller's Start), and the skin's menu capsule holds lamps for the buttons a controller has and the pad does not (L2, R2, L3, R3, Home). Only when no skin can be read does it fall back to the plain diagram, with the readouts under the card. Portrait and landscape. The connection is only named when the system can confirm it: Android says USB when a USB device with the controller's vendor and product id is attached to the phone, Bluetooth when a paired Bluetooth device has the controller's name (needs `BLUETOOTH_CONNECT` on Android 12+), and otherwise shows just the controller's name; a cable that only charges a Bluetooth controller does not make it USB. iOS says wired only when GameController reports the controller as attached to the device (`isAttachedToDevice`) and Bluetooth otherwise: a Switch Pro Controller plugged into an iPhone by USB-C usually keeps talking over Bluetooth, the cable only charges it.

## App version

Home (small, faint) and the bottom of Settings show **go-link Console Player v*X.Y.Z* (build *N*)**, plus " · debug" in debug builds (`app-version`; a long press copies it). The build number follows one rule on both platforms: major × 10000 + minor × 100 + patch (0.1.5 → 105, 1.2.3 → 10203).

- **Android:** `BuildConfig.VERSION_NAME` / `VERSION_CODE`, from `-PversionName` / `-PversionCode` (`app/build.gradle.kts`, defaults 0.1.0 and 1). A release uses `VERSION=0.1.5 make android-apk` (`scripts/release.sh` does), which computes the code.
- **iOS:** `CFBundleShortVersionString` / `CFBundleVersion` come from the build settings `MARKETING_VERSION` / `CURRENT_PROJECT_VERSION` (defaults 0.1.0 and 1 in `Config/Base.xcconfig`). `make build-sim|device-build|test-ui VERSION=0.1.5` computes build 105, or pass `MARKETING_VERSION=0.1.5 CURRENT_PROJECT_VERSION=105` to `xcodebuild`.

## Permissions

Each one is asked when needed, after a short explanation screen:

| Permission | When | Without it |
|---|---|---|
| `CAMERA` | Opening the QR scanner | Type the code or paste the link; the scanner offers the system settings after a denial |
| `RECORD_AUDIO` | The first time the microphone is turned on | Play and listen; the mic button says it is blocked and opens the explanation again |
| `BLUETOOTH_CONNECT` (Android 12+) | Before the first room | Bluetooth headsets are not used; wired and USB headsets still are |
| `MODIFY_AUDIO_SETTINGS`, `INTERNET` | Install time (normal permissions) | |

**Audio route** (`audio/AudioRouter.kt`): while a room is open the app uses the communication audio mode, which gives echo cancellation and lets a headset's microphone work. It picks, in order, a Bluetooth LE headset, a Bluetooth SCO headset, a hearing aid, a USB headset or wired headphones, and otherwise the **loudspeaker** (never the earpiece). Android 12+ uses `setCommunicationDevice`; older versions use Bluetooth SCO and the speakerphone switch. It follows headsets being plugged in or out. Over Bluetooth, the communication mode means the headset's call profile (mono, lower quality than music), which is the price of using its microphone. Leaving the room restores the previous mode. That is **Automatic**; the Sound sheet can pick the devices by hand.

### Sound devices

The rules live in `:core` (`AudioDevices.kt`, tested on the JVM): the rows, a saved choice mapped back to a live device, the route plan, and when a chosen device is gone. `audio/AudioRouter.kt` lists the platform devices and applies the plan.

- **Saved:** both choices are kept in SharedPreferences (`go-link.audio-output`, `go-link.audio-input`) by kind, product name and address (Bluetooth address, USB path), since Android's device ids change when a device reconnects. The phone's own speaker and microphone are saved by kind only.
- **Absent at the start:** a saved headset that is not connected when the room opens waits: the route stays automatic, the sheet marks Automatic, and the headset is used as soon as it connects.
- **Disconnected:** a chosen device that was connected and then disappears (`AudioDeviceCallback`) sends that choice back to Automatic, forgets it, and a snackbar says "«NAME» disconnected. Sound went back to Automatic." (or "The microphone went back to Automatic.").
- **Output:** a chosen output is the communication device. A chosen headset microphone with an automatic output also makes that headset the communication device, since Android opens a Bluetooth headset's microphone only together with its sound.
- **Microphone:** a chosen microphone is also given to libwebrtc's recorder as its preferred device (`JavaAudioDeviceModule.setPreferredInputDevice`), which is what lets the phone's microphone be used while the sound goes to a headset. Android treats a preferred device as a request, not a rule, and a few phones ignore it in the communication mode.

API levels:

| | Android 12+ (API 31) | Android 8 to 11 (API 26 to 30) |
|---|---|---|
| Output list | `availableCommunicationDevices` | `getDevices(GET_DEVICES_OUTPUTS)` (Bluetooth as its SCO device, no hearing aids) |
| Choosing an output | `setCommunicationDevice` with that exact device | By kind only: Bluetooth starts SCO, Speaker turns the speakerphone on, wired and USB turn both off (with two headsets of the same kind Android picks) |
| Microphone | Follows the communication device, plus the preferred input | The same preferred input; a Bluetooth microphone needs SCO, so it brings the Bluetooth output with it |
| Bluetooth devices | Listed only with `BLUETOOTH_CONNECT` | Listed without it |

Limits: a Bluetooth microphone cannot be used while the sound plays on the loudspeaker or another headset (Android opens it only with its own output, in the call profile). Device addresses are read on Android 9+ (API 28); older versions match a headset by kind and product name.

## Startup intro and the room code

- **INSERT COIN intro** (both apps, only on a cold start: not when coming back from the background or after a rotation). The first frame is the plain #05060A background (Android: the SplashScreen compat API, `Theme.GoLink.Starting`, with a transparent icon; iOS: the static launch screen, `LaunchBackground`), then the animated intro (`ui/IntroScreen.kt`, `UI/IntroView.swift`): the icon pops in (0.3 → 1.12 → 1 in 0.7 s) with an orange glow, "go-link" rises at 0.6 s, **INSERT COIN** rises at 1.1 s with the coin sound and blinks from 1.5 s; it fades out at 2.0 s. A tap skips it (before 1.1 s, silently). With reduced motion (Android "remove animations", iOS Reduce Motion) everything just fades in. INSERT COIN is 5x7 pixel letters drawn in code from `PixelText` (`:core` / GoLinkCore, the same letters as the website's inline SVG), read as "INSERT COIN" in every language.
- **Coin sound**: `res/raw/coin.wav` and `Resources/coin.wav`, written by `node scripts/coin-sound.mjs` (three square-wave notes, 44.1 kHz mono 16-bit; an original sound, MIT like the project, the same synthesis the website plays with Web Audio). Android plays it with SoundPool (`USAGE_GAME`, no audio focus) and skips it on silent or vibrate; iOS with AVAudioPlayer in the `.ambient` session (the silent switch mutes it, other audio keeps playing; the room's audio device sets its own session when it starts). **Settings › Startup sound** (on by default; Android SharedPreferences `startup-sound`, iOS UserDefaults `go-link.startup-sound`) turns it off.
- Debug builds open labs for screenshots with no room: Android `--es lab alias` (the name step) and `--es lab overlay` (stats box, see-through pad, pause banner); iOS `-aliasLab` and `-padLab -labStats -labGhost`.
- Debug builds slow the intro down to look at it: Android `adb shell am start -n org.golink.player/.MainActivity --es introSlow 5`, iOS `-introSlow 8`; iOS `-noIntro` skips it (the UI tests use it).
- **Room code field**: the Join screen shows the 9 digit code grouped as `915 355 636` while it is typed (`CodeInput` in `:core` and GoLinkCore, the same rules and tests as the website's `editCode`): the spaces are only visual, the caret stays after the same digit, backspace right after a space deletes the digit before it, and a paste with or without spaces works. A pasted invitation link is not grouped. The field also takes links, so it keeps the normal keyboard.

## App Links

The app declares `https://play.go-link.org/g/*` with `autoVerify`. Android trusts it only when `https://play.go-link.org/.well-known/assetlinks.json` lists the app's signing certificate. The file is in `frontend/apps/web/public/.well-known/assetlinks.json` with the release key's fingerprint; if the key ever changes, update it before the next web deploy (steps in the [app README](../mobile/android/README.md#app-links-fingerprint)). The host must serve it as `application/json`, without redirects. Until it is verified, Android asks which app should open the link.

On Android, the website's `/g/:invite` page shows a small card under the PIN form (`components/AndroidAppCard.tsx`) linking to the latest release, with the browser as the other way to play.

## Tests and CI

- `./gradlew :core:test`: protocol parsing, input packets against vectors produced by the website's `encodeInput`, invitation parsing (hostile QR codes), signaling URLs, ICE servers, the terms version, room passes, the Sound sheet's device choices (rows, saved choices after a reconnect, the route plan, a chosen device disconnecting), the name rules (emoji, symbols, accents, ñ, spaces, lengths, sanitizing), the pause request fields (`host_online`, `pause_asked`, `pause_declined`), the stats arithmetic and the latency meter, the screen rate (fastest mode at the same resolution, the measured rate with missed frames and pauses, the labels), and a full `RoomClient` visit with a fake signalhub and a fake peer (join, PIN, offer/answer, candidates, control, the cleaned name in `hello`, `pause_request` and its cancel, input repeats, drop and return with the token, refused token, host leaving).
- `./gradlew :app:assembleDebug` builds the app. CI runs both (`android` job in `.github/workflows/ci.yml`).
- `cd e2e && npm run test:android`: the app end to end on an emulator or phone (below).
- `cd e2e && npm run test:android:camera`: the QR scanner with the emulator's virtual camera (opt-in, below).
- Not covered by automated tests: a real game with ROMs (the e2e uses the test pattern room), a real camera and microphone, real headsets and controllers.

### End-to-end test on an emulator

`e2e/tests/android.spec.ts` (Playwright project `android`) starts the usual e2e stack (its own signalhub on 8191, a headless device with `--debug` and its own HOME, the website on 5191). A Chromium page with a fake microphone is the owner: it links the device, opens the test pattern room (taking P1) and makes the invitations. The debug APK, driven with `adb` and UiAutomator (Compose test tags exposed as resource ids: `join-pin`, `terms-check`, `room-seat`, `dock-mic`, `pad-dpad`, `chat-input`…), then:

1. points the app at `ws://10.0.2.2:8191/ws` in Settings (debug builds only);
2. joins by App Link (`am start -d https://play.go-link.org/g/<invite>`), types the PIN, ticks the terms, confirms a name on the name step (`alias-field`, `alias-enter`; after every new admission in the test), gets a seat, and the picture area of a screenshot is lit and varied (not black);
3. presses the D-pad, button 1 and Coin: the device's debug log shows each `input` packet;
4. game sound: inbound RTP packets and audio energy grow and the audio level is above 0 (decoded, not only received);
5. the Sound sheet (players tab › **Sound**) lists Automatic for the microphone and the output and the Speaker row, **Test sound** plays, and Back closes it;
6. chat both ways (owner → app, app → owner);
7. voice both ways: the owner's fake microphone reaches the app's `voice-pN` track; with `RECORD_AUDIO` granted (`pm grant`), the app's microphone reaches the owner's `voice-pN` track (packets flowing; the emulator's microphone may be silence);
8. airplane mode on and off: the app comes back to its seat with the return token, never asking the PIN;
9. a wrong PIN and an already used PIN are refused (`used`), then a new invitation's PIN gets in;
10. joins with the 9-digit code typed on the home screen;
11. the start buttons: with two seated players the pad shows exactly 1P and 2P; holding each one reaches the device (`buttons=start1`, `start2` in its log) and lights that player lamp of the test card while held (the lamp's color is read from the screen where `pkg/testpattern` draws it);
12. seats and the queue: the app leaves, three browser guests (headless Chromium, each with its own invitation, through the website's Join a game form) take P2 to P4, and the app joining next waits in the queue ("You are #1 in the queue", `me=queue:1`) while it still gets the picture; when a guest leaves, the app takes exactly the freed seat; **Just watch** makes it a spectator (listed under Watching) and **Join the queue** seats it again;
13. swapping seats: the app asks a guest (`swap_seat`, "Waiting for P*n* to answer") and the guest accepts on the website (`swap_answer`); then the guest asks from its players capsule and the app accepts the offer over the picture. Both sides' seats are checked (the app's `GoLinkE2E` state, the guest's own avatar in the capsule).

The app's debug build logs its room events and WebRTC counters (inbound/outbound RTP per track, audio levels, the candidate pair) once a second with the tag `GoLinkE2E` (`app/src/debug/.../E2eProbe.kt`; the release build has an empty probe in `app/src/release`), and the test reads them with `adb logcat -s GoLinkE2E`. Audio is proven with these counters: `screenrecord` records video only. The screen is recorded in chunks (`screenrecord`, 175 s each) and a screenshot is saved per step, all in `e2e/test-results/android/` (and copied to `E2E_EVIDENCE_DIR` when set).

The harness (`e2e/android.ts`) closes system dialogs that a slow emulator shows over the app while it waits for a node: "*X* isn't responding" gets **Wait**, another app's crash gets **Close**, and a crash of go-link Console Player fails the test with the dialog's text. A release build on the device (another key, a higher version code) is uninstalled before the debug build is installed; reinstall the release APK afterwards if you need it.

ICE with the emulator works without TURN or `--announce`: the emulator sends its checks to the device's host candidate through the emulator's NAT, and the device learns a peer-reflexive candidate (`GoLinkE2E` shows `prflx->host`).

```bash
cd mobile/android && ./gradlew :app:assembleDebug     # the debug APK
emulator -avd <name> -no-window -no-snapshot -gpu swiftshader_indirect &   # keep audio on
cd ../../e2e && npm run test:android
```

The emulator's microphone stays silence: `-allow-host-audio` would use the computer's real microphone, and the emulator's gRPC `injectAudio` (to feed it a tone) crashed emulator 37.1.11 on macOS every time it was tried, so the voice check proves packets from the app, not their level.

Without a device in `adb devices` the project is skipped; `npm test` runs only the web project. `ANDROID_SERIAL` picks a device, `ANDROID_APK` another APK, and `ANDROID_SIGNAL_URL` another server (a phone can use `adb reverse tcp:8191 tcp:8191` and `ws://127.0.0.1:8191/ws`). `.github/workflows/android-e2e.yml` runs it weekly and by hand on an emulator with KVM. `E2E_CHROMIUM_SINGLE_PROCESS=1` runs Chromium as one process, only for a local macOS session where Chromium cannot start.

### The QR scanner on the emulator's camera

`cd e2e && npm run test:android:camera` (Playwright project `android-camera`, `e2e/tests/android-camera.spec.ts`) needs the Android emulator, not a phone. The emulator's `virtualscene` back camera renders a 3D room with a poster on a wall (its place is in the SDK's `emulator/resources/Toren1BD.posters`); `adb emu virtualscene-image wall <png>` changes the poster's picture, and the emulator's gRPC (`setPhysicalModel`, with the token from its discovery file `pid_<pid>.ini`) moves the virtual device in front of it. The test:

1. restarts the emulator with its own command line plus `-camera-back virtualscene` when it does not already use it (`adb emu kill`, then the `emulator` launcher from the discovery file), and at the end starts it again exactly as it was;
2. links the owner, opens the test pattern room, points the app at the test signalhub and makes an invitation;
3. draws two QR codes with the website's encoder (`qrcode-generator`, level M, as `QrCode.tsx`): the website's own link on the test stack (not a go-link invitation for the app) and the same invitation as `https://play.go-link.org/g/<invite>`;
4. opens **Scan QR code** from home, taps **Allow** on the explanation and **While using the app** on the system's camera dialog; the scanner sees the first code and refuses it ("That QR code is not a go-link invitation.");
5. puts the real invitation on the wall: the scanner reads it, the PIN screen opens with that invitation, and the PIN and the terms join a seat.

Its screenshots and screen recording go to `e2e/test-results/android-camera/` (and `E2E_EVIDENCE_DIR`).

## Releases

`VERSION=x.y.z make android-apk` runs the tests and builds the signed release APK into `dist/android/`. `VERSION=x.y.z make release` builds it too (`ANDROID=0` skips it) and publishes it as `go-link-console-vX.Y.Z-android.apk`; the website's Android card downloads that file and the guide points to the latest release. The release key lives outside the repository (`ANDROID_SIGNING`, see the [app README](../mobile/android/README.md)).

## The iOS app

`mobile/ios/` is **go-link Console Player for iPhone and iPad**: SwiftUI, iOS 17 or newer, bundle id `org.golink.player`. Same features and rules as the Android app. It is not distributed yet: the website keeps iPhone and iPad as "coming soon". Build, sign and install: [`mobile/ios/README.md`](../mobile/ios/README.md).

| Part | What it is |
|---|---|
| `GoLinkCore` | A Swift package ported one to one from `:core` (no UIKit, no WebRTC; `swift test` on the Mac): `SignalClient`, `RoomClient`, `Invites`, `SignalUrls`, `IceServers`, `InputPacket` (byte for byte the web's `encodeInput`, same test vectors), `RoomMessages`, `RtcSignals`, `RoomPasses`, `Terms`, `AudioChoices`. Timers go through a `Scheduler`, so the tests run on a manual clock. |
| The app | SwiftUI screens, `URLSessionWebSocketTask`, `IOSRtcPeer` on the prebuilt libwebrtc of [stasel/WebRTC](https://github.com/stasel/WebRTC) (Swift Package Manager), AVFoundation for the QR code, GameController for controllers, UserDefaults (`go-link.room-passes`, `go-link.terms`, `go-link.signal-url`, as on Android). The Xcode project is generated with XcodeGen from `project.yml`. |

Differences from Android:

- **Audio device.** libwebrtc's own iOS audio device opens the microphone as soon as there is sound to play, and libwebrtc starts "recording" as soon as the microphone line is negotiated, even with no track. The app plugs in its own `RTCAudioDevice` (`GoLinkAudioDevice`): a plain output unit with the playback session while listening, and the voice processing unit (echo cancellation) with play and record, `voiceChat`, `defaultToSpeaker` and Bluetooth HFP + A2DP only while the person's microphone switch is on. The microphone permission is asked the first time the microphone is turned on, and sound never goes to the earpiece.
- **Output.** iOS apps can offer the system's route picker (`AVRoutePickerView`), so **Game settings** shows it for speaker, headphones, AirPods or AirPlay; the microphone is chosen from the session's inputs (Automatic, the iPhone's or a headset's) with `AudioSelection` from GoLinkCore. The panel also has the game and voices volumes (0 to 100 %) and a test chime.
- **Drawer.** The room's drawer has the three tabs of the approved design, **Chat** (typing indicator, unread badge), **Players** (seats, move, swap, silence, watch or queue) and **You** (seat, Game settings, How to play, Leave); it slides in from the right sideways and from the bottom upright.
- **Controllers.** MFi, Xbox, PlayStation and Switch Pro through GameController's extended gamepad, by position like the standard layout (A bottom, B right, X left, Y top; Options = Select = Coin, Menu = Start); each controller takes the next local player and the on-screen pad steps aside.
- **Universal Links** instead of App Links: `applinks:play.go-link.org`, only `/g/*`, only the invitation is read. The website serves `frontend/apps/web/public/.well-known/apple-app-site-association` as `application/json`; its `appIDs` has the placeholder `TEAMID00000.org.golink.player`, to be replaced with the signing team before a deploy.
- **Debug builds** accept `ws://` to `localhost` and `127.0.0.1` only (the Simulator reaches the Mac through them), and the launch arguments `-invite <link>`, `-signal <url>`, `-silentOutput` and `-resetState`. `-aliasLab` opens the name step alone, `-pictureLab` streams the website's synthetic test card (384 × 224 at 60 fps, shown at 4:3) into the real renderer with the dock's gear and the Game settings panel (`-labStyle`, `-labBands`, `-labCompare`, `-labSettings`, `-labUp2`, `-labRaw`; `picture-lab-fps` shows the new frames drawn per second and the screen rate), and `-padLab` takes `-labStats` (the stats box with sample values) and `-labGhost` (the see-through pad with a sample controller), for screenshots. Their probe logs the room and libwebrtc's counters to the unified log (subsystem `org.golink.player`, category `e2e`), like Android's `GoLinkE2E`.

Tests: `make test-core` (unit tests mirroring the Android ones, plus the terms version check against `legal.ts`) and the XCUITest target (`testScreens` for screenshots; `testLiveJoin` joins a real test pattern room given `TEST_RUNNER_GL_INVITE` and `TEST_RUNNER_GL_PIN`: PIN, terms, video and game sound, pad presses on the `input` channel, chat round trip, the drawer tabs, the sound sheet, the microphone prompt and landscape; it passes the name step), `testAliasValidation` (an emoji disables Enter with the red hint, a valid name enables it), `testControllerTestScreen` (Home › Test controller opens and closes, and measures the screen rate), `testStatsAndGhostPadLab`, `testPicture2xMatchesNative` (the offscreen 2x check, then the lab streaming 2x frames), `testPictureRoomDefault` (a room default applies until the viewer picks, and Use the room's default brings it back) and `testGameSettingsAndPictureStyles` (picture lab: the gear opens Game settings, every style and side can be chosen, Compare shows the divider, portrait and landscape; with `TEST_RUNNER_GL_SHOTS` it saves every style × side). CI (`ios` job on `macos-latest`) runs the GoLinkCore tests and an unsigned Simulator build.
