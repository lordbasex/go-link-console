<!-- Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> -->
# go-link Player for iOS

The native iPhone and iPad app for joining a go-link game as a player: the iOS counterpart of [the Android app](../android/README.md). SwiftUI, iOS 17 or newer, bundle id `org.golink.player`. It speaks the same signalhub and WebRTC protocol as the website ([docs/mobile.md](../../docs/mobile.md), [docs/protocol.md](../../docs/protocol.md)). It only joins rooms; hosting stays on the device app.

It is distributed through **TestFlight** (Apple's beta: a public link for friends once Apple approves each build, see [TestFlight](#testflight)), and you can also build it and install it on your own iPhone as explained below.

## Layout

| Path | What it is |
|---|---|
| `GoLinkCore/` | A Swift package with the portable logic, a port of the Android `:core` module: the signalhub envelope and `SignalClient` (jittered backoff, FIFO requests, ICE servers from `hello` in memory only), invitation parsing, the signaling URL rules, the 12-byte `input` packet (byte for byte the web's `encodeInput`), the touch pad and controller mapping, `room_state` / `chat` / `typing` / `stream_stats`, the PIN gate, the return tokens (`RoomPasses`), the microphone choices (`AudioDevices`) and `RoomClient`, which runs one room visit. No UIKit and no WebRTC: `swift test` runs it on the Mac. |
| `GoLinkPlayer/` | The app: SwiftUI screens, `URLSessionWebSocketTask`, `IOSRtcPeer` (libwebrtc), its own audio device (`GoLinkAudioDevice`), the audio route, GameController controllers, the QR scanner (AVFoundation), UserDefaults. |
| `GoLinkPlayerUITests/` | XCUITest: screenshots of the main screens, the on-screen pad across rotations, and a live join when given an invitation. |
| `project.yml` | The [XcodeGen](https://github.com/yonaskolb/XcodeGen) spec. `GoLinkPlayer.xcodeproj` is generated and not committed. |
| `Config/` | Build settings; your Apple team goes in the gitignored `Local.xcconfig`. |

Dependencies are Swift packages only: [stasel/WebRTC](https://github.com/stasel/WebRTC) (a prebuilt libwebrtc, pinned in `project.yml`) and the local `GoLinkCore`. No CocoaPods.

## Build and run

Needs Xcode 26 (the project targets iOS 17) and XcodeGen (`brew install xcodegen`).

```bash
cd mobile/ios
make project        # writes GoLinkPlayer.xcodeproj
make test-core      # GoLinkCore unit tests (swift test)
make build-sim      # Simulator build, no signing (SIM="iPhone 17" by default)
open GoLinkPlayer.xcodeproj
```

In Xcode pick the **GoLinkPlayer** scheme and a Simulator, then Run. The Simulator has no camera: the scanner explains the permission and then offers to type the code.

UI tests (`make test-ui`, or with `xcodebuild test`). Values reach the test runner as `TEST_RUNNER_`-prefixed environment variables:

```bash
TEST_RUNNER_GL_LANG=es TEST_RUNNER_GL_SHOTS=/tmp/shots \
  xcodebuild test -project GoLinkPlayer.xcodeproj -scheme GoLinkPlayer \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -only-testing:GoLinkPlayerUITests/PlayerUITests/testScreens

# A live join (a test pattern room invitation and its PIN from the website):
TEST_RUNNER_GL_INVITE=https://go-link.org/g/<invite> TEST_RUNNER_GL_PIN=<pin> \
  xcodebuild test ... -only-testing:GoLinkPlayerUITests/PlayerUITests/testLiveJoin
```

`testLiveJoin` types the PIN, accepts the terms, joins, checks that no system prompt appears for just listening, presses Coin, 1P, a button and the D-pad, sends a chat line and waits for it to come back, opens Players, You and Game settings (switches to CRT arcade), turns the microphone on (the app's explanation, then the system's prompt) and turns the phone sideways.

### Debug-only helpers

Debug builds (never release builds) accept:

- `-invite https://go-link.org/g/<invite>` as a launch argument: opens the PIN screen for that invitation, through the same strict parser as Universal Links (only the invitation is read). Universal Links need the published association file and a signed app, so this is how the Simulator opens an invitation: `xcrun simctl launch booted org.golink.player -invite https://go-link.org/g/<invite>`.
- `-signal ws://127.0.0.1:8090/ws`: a signaling server; debug builds also accept `ws://` to `localhost` and `127.0.0.1` in Settings, for a local signalhub.
- `-silentOutput`: computes the level of the sound it would play (for the probe) but plays silence, for automated runs on a desk.
- `-padLab`: opens a pad lab instead of the app: the room's console layouts with the on-screen pad and no room, listing every held-buttons value sent (`testPadAcrossRotations` rotates it and presses the pad).
- `-resetState`: forgets the accepted terms, the name, the custom server and the return tokens.
- `-pictureLab`: the website's synthetic test card (384 × 224, 60 fps, 4:3) streamed into the real picture renderer, with the dock's gear and the Game settings panel; `-labStyle smooth|sharp|crt|edges`, `-labBands black|ambient|frame`, `-labCompare`, `-labSettings` and `-labRoom crt,frame` (a room default) set it up for screenshots (`testGameSettingsAndPictureStyles` and `testPictureRoomDefault` use it; with `TEST_RUNNER_GL_SHOTS` exported it saves every style × side in portrait and landscape). `-plainPicture` forces the fallback view. `-labUp2` streams the card enlarged 2x with nearest neighbour as a 2x stream (`-labRaw` draws it raw).
- `-picture2xCheck`: draws the test card offscreen in every style and side, at its own size and as a 2x stream, and checks the bytes are identical (`testPicture2xMatchesNative`; see [docs/mobile.md](../../docs/mobile.md#game-settings-and-picture-styles)).
- `-aliasLab`: opens the name step alone (`testAliasValidation` uses it); `-padLab` also takes `-labStats` (the stats box with sample values) and `-labGhost` (the see-through pad with a sample controller chip).
- `-noIntro`: no startup intro (the UI tests pass it); `-introSlow <factor>` plays the intro that many times slower, to take screenshots of its frames.
- The **probe**: the room's events and libwebrtc's counters (inbound RTP per track, outbound microphone RTP, data channel messages, the candidate pair, the played level, whether the microphone is open) as one JSON line per second in the unified log. It never logs a PIN or a token:

  ```bash
  xcrun simctl spawn booted log stream --level info \
    --predicate 'subsystem == "org.golink.player" AND category == "e2e"'
  ```

## Startup intro

On a cold start (never when coming back from the background) the app plays a short intro over its first screen, like the website's loading screen: the go-link mark pops in, "go-link" rises, then "INSERT COIN" (pixel lettering drawn in code, `GoLinkCore.PixelText`) rises with a coin sound and blinks; about two seconds, and a tap skips it. With Reduce Motion it only fades. The static launch screen has the intro's first color. The coin (`GoLinkPlayer/Resources/coin.wav`) is an original sound made by `scripts/coin-sound.mjs` (MIT like the rest of the project); it plays with the **ambient** audio session, so the silent switch mutes it and other apps' audio keeps playing. Settings › **Startup sound** turns it off.

The room code field shows the code grouped while you type (`915 355 636`, `GoLinkCore.CodeInput`); it also takes a pasted invitation link, left as it is.

## Name, pause, stats and controllers

- After the PIN the app asks **What's your name?** (2 to 20 letters, digits and spaces, no symbols or emoji; `GoLinkCore.PlayerName`, the device's rules) and remembers it; the You tab and Settings use the same field.
- Only the host pauses: the dock's pause button **asks for a pause** (`pause_request`) and can cancel it; it explains when the host is away or the game cannot pause.
- The round **stats** button at the top left of the picture shows fps, resolution and codec, ping, direct or relay, loss and audio, read from libwebrtc's statistics once a second (`Rtc/LiveStatsReader.swift`, `GoLinkCore.LiveStatsMeter`), plus the measured screen refresh rate (`screen 120 Hz`) and what the device sends (`video 768×448 (2× of 384×224)`, `quality High`, from `stream_stats.video`).
- **ProMotion (120 Hz):** `CADisableMinimumFrameDurationOnPhone` is on, and the room and Test controller run a `CADisplayLink` with `CAFrameRateRange(minimum: 60, maximum: 120, preferred: 120)` (`UI/ScreenRateMonitor.swift`, `GoLinkCore.RefreshRateMeter`); the video's `MTKView` checks for new frames 120 times a second. Menus stay at the system's rate to save battery.
- With a real controller connected, the gamepad button shows the on-screen pad **see-through and display only**, lighting what you press on the controller. **Test controller** on Home checks a controller offline, with the input latency (to the next `CADisplayLink` frame, about one screen frame) and the screen's refresh rate. The link says wired only when GameController reports the controller attached to the device (`isAttachedToDevice`); a Switch Pro on USB-C usually still talks Bluetooth.

Details: [docs/mobile.md](../../docs/mobile.md#your-name).

## Group invitations, raised hands and voice

- **Group and spectators' invitations:** a link `https://go-link.org/g/<invite>#k=<key>` (Universal Link, QR code or pasted) carries a key after `#k=` (exactly 43 base64url characters, otherwise ignored; `GoLinkCore.Invites.scanned` / `typed` / `appLink` return an `Invitation`). With a key the join form asks a **name** for the host (prefilled with your saved name) and the terms instead of a PIN, and `RoomClient` sends `{kind: "pin", key, name}` (name 24 characters at most) on every ask. While the host decides (`pin_result` reason `waiting`) the room shows **Waiting for the host** with Leave; a refusal (declined, full, expired, busy, wrong, blocked, locked) shows why and stops. Once in, the return token is saved as with a PIN, and a dropped connection comes back with it. The key lives only in memory: never logged or saved.
- **Taken out by the host:** `pin_result` reason `removed` ends the visit ("The host took you out of this room"); nothing is tried again and the token is kept (it would only answer `removed`).
- **Raised hands:** a spectator gets a ✋ button in the dock (`{type: "hand", on}`), dimmed while the host is not in the room. Someone the host moved to watching (`room_state.you.held`) raises a hand in Players instead of **Join the queue**. Short notices explain the device's refusals (`hand_too_soon` with the seconds left from `you.hand_again`, `hand_no_host`, `queue_ask_host`) and the host's "not now" (chat `hand_later`); the chat translates `let_queue`, `play_now`, `first_in_queue`, `removed`, `sent_to_queue` and `sent_to_watch`.
- **Voice:** the players and the queue talk; spectators listen (the device enforces it, and the app only sends the microphone while it is a player or in the queue). The four voice tracks are slots: `room_state.voice_slots` says who each carries (`GoLinkCore.VoiceSlots`), the tracks' levels from libwebrtc's statistics light up the speaker's seat or queue row in Players, and **Silence** mutes a person by name, whichever slot carries them.
- **The voice question:** each time the app becomes able to talk with its microphone off in a room with voice, a card under the picture asks **Do you want to join the voice?** (Turn on my microphone / Not now; `GoLinkCore.VoiceAsk`), never together with the name step.

## Version

Home and Settings show **go-link Player v*X.Y.Z* (build *N*)** (" · debug" in debug builds; a long press copies it). `CFBundleShortVersionString` and `CFBundleVersion` come from the build settings `MARKETING_VERSION` and `CURRENT_PROJECT_VERSION` (defaults 0.1.0 and 1 in `Config/Base.xcconfig`). A release passes the version: `make device-build VERSION=0.1.5` (also `build-sim` and `test-ui`) computes build 105 with the Android rule (major × 10000 + minor × 100 + patch), or give both to `xcodebuild` (`MARKETING_VERSION=0.1.5 CURRENT_PROJECT_VERSION=105`).

## Sound and the microphone

libwebrtc's built-in iOS audio device opens the microphone as soon as there is sound to play. The app plugs in its own (`GoLinkAudioDevice`, through `RTCAudioDevice`): while you only listen it plays through a plain output unit with the **playback** session (full quality, speaker, wired headphones or AirPods over A2DP); only while your microphone is on it switches to the voice processing unit (echo cancellation) with **play and record**, `voiceChat`, `defaultToSpeaker` and Bluetooth HFP + A2DP. So the microphone permission is asked when you first turn the microphone on, never just for joining, and sound never goes to the earpiece.

**Game settings** (the dock's gear, or drawer › You) has your name, the game and voices volumes (0 to 100 %), the picture (style Smooth, Sharp, CRT arcade or Smooth edges; sides Black, Ambient or Frame; Compare with a draggable line; drawn by our Metal port of the website's shaders in `GoLinkPlayer/Picture/`, which averages a 2x stream back to the game's pixels first; the room's default from its host applies until you pick your own, and **Use the room's default** goes back to it; see [docs/mobile.md](../../docs/mobile.md#game-settings-and-picture-styles)), the stats switch, the microphone (Automatic, the iPhone's, or a headset's, with `setPreferredInput`) and the output through the system's route picker (`AVRoutePickerView`: speaker, headphones, AirPods, AirPlay), plus a test chime.

## Sign and install on your iPhone

Nothing about signing is committed. Put your team in a local file:

```bash
cp Config/Local.xcconfig.example Config/Local.xcconfig
# edit it: DEVELOPMENT_TEAM = <your 10-character Team ID>
make project
```

Find the Team ID in Xcode › Settings › Accounts (select your Apple ID and the team), or on developer.apple.com › Membership.

- **Free Apple ID** (a "Personal Team"): works for your own devices. Apps expire after **7 days** (run from Xcode again to renew), and a personal team cannot use Associated Domains: uncomment `GOLINK_ENTITLEMENTS =` in `Local.xcconfig` to build without Universal Links (invitation links then open the website; scan the QR code or type the code in the app instead).
- **Paid developer account**: builds last a year and Universal Links work once the association file is published (below).

Then:

1. Connect the iPhone with a cable (or pair it over Wi-Fi in Xcode › Window › Devices and Simulators) and trust the computer.
2. On the iPhone turn on **Developer Mode**: Settings › Privacy & Security › Developer Mode, restart and confirm (it appears after the phone has been connected to Xcode once).
3. In Xcode pick your iPhone as the destination and Run (or `make device-build`). Xcode creates the provisioning profile (Automatic signing).
4. With a free Apple ID, the first launch is blocked until you trust the developer: Settings › General › VPN & Device Management › your Apple ID › Trust.

## Automated signing (App Store Connect API key)

With a paid account, an [App Store Connect API key](https://appstoreconnect.apple.com/access/integrations/api) (role Admin, the `.p8` file Apple lets you download once) lets `xcodebuild` create the certificates and provisioning profiles, register the connected iPhone and upload builds by itself, without Xcode's Accounts. Nothing of it is committed: keep the key outside the repository and pass it through the environment, for example a file you `source` first:

```bash
export TEAM_ID=...            # developer.apple.com › Membership
export ASC_KEY_ID=...         # the key's ID
export ASC_ISSUER_ID=...      # Users and Access › Integrations › Issuer ID
export ASC_KEY_PATH=/path/to/AuthKey_<ASC_KEY_ID>.p8
```

| Command | What it does |
|---|---|
| `make install-device` | Debug build signed for the first paired iPhone (cable or Wi-Fi), installed and launched with `devicectl` |
| `make testflight` | Release archive (`VERSION` from the latest tag, build number `<version code>.<yymmddHHMM>` so every upload is new), exported with `method app-store-connect` and uploaded to App Store Connect |
| `scripts/asc.py GET\|POST\|PATCH /v1/...` | The App Store Connect API with the same key (no packages: openssl signs the token), for TestFlight groups, testers and beta review |

`GoLinkPlayer/PrivacyInfo.xcprivacy` is the privacy manifest the App Store requires: no tracking, no data collected, and the reasons for the two "required reason" APIs the app uses (`UserDefaults` CA92.1, system boot time 35F9.1).

## TestFlight

App Store Connect has the app **go-link Player** (bundle id `org.golink.player`, SKU `golink-player`) and two TestFlight groups:

- **Equipo** (internal): the team's own App Store Connect users, every build as soon as Apple processes it (about 10 minutes after `make testflight`), no review.
- **Amigos** (external): anyone with the public link, up to 100 people. Each new build is added to the group and submitted to Beta App Review (`betaAppReviewSubmissions`); once approved, the link installs it through the TestFlight app. The beta's description, feedback email, review contact and What to Test are set in English, Spanish and Portuguese.

A tester installs **TestFlight** from the App Store, opens the link (or the email invitation) and taps Accept, then Install. Builds expire after 90 days.

## Universal Links

The app claims `https://go-link.org/g/*` (`applinks:go-link.org` in `GoLinkPlayer/GoLinkPlayer.entitlements`, generated from `project.yml`). Only the invitation (and a group invitation's key after `#k=`) is read from the link, never a PIN or a signaling server.

The website publishes `frontend/apps/web/public/.well-known/apple-app-site-association` (JSON, no extension). Its `appIDs` holds a placeholder, `TEAMID00000.org.golink.player`: replace `TEAMID00000` with the Team ID that signs the app before deploying the website. The CDN must serve the file at `https://go-link.org/.well-known/apple-app-site-association` with `Content-Type: application/json`, without redirects. Apple's CDN caches it; while developing, `applinks:go-link.org?mode=developer` in the entitlements (with the device's Developer Mode on) makes the phone fetch it directly.

## What it does

Home (Scan QR code, Type the room code, Paste the invitation link), the QR scanner, the PIN screen with the terms (`Terms.version` equals `TERMS_VERSION` in `frontend/apps/web/src/legal.ts`, a test checks it), Settings (your name, your own signaling server: `wss://` only, tested for `hello` before saving, a notice and **Back to the official server** while it is active), and the room: the game in portrait (Game Boy) and landscape (Switch) console layouts, the on-screen gamepad from `room_state.controls` with Coin and 1P…NP, MFi, Xbox, PlayStation and Switch Pro controllers (GameController, mapped by position like the website's standard layout; each one takes the next local player), and the drawer with **Chat** (typing indicator, unread badge), **Players** (seats, move, swap, silence, watch or queue) and **You** (seat, Game settings, How to play, Leave), plus the name step, pause requests, the stats overlay, the see-through pad and **Test controller** (above). The rules are the Android app's: every new join asks for the PIN (or, for a group invitation, a name); the return token is only used to get back in after a drop during the same visit.
