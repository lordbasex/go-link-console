# How it works: main flows

## 1. First run of the device

1. The host downloads and opens go-link Console (double click, no terminal).
2. The device creates `device.json` with its `device_id` and `device_secret`, and opens its **window**.
3. It connects to the signaling server and gets a **9-digit pairing code**, shown in the window, in the tray panel and (headless) in the log.
4. The host opens the website (`https://play.go-link.org/device` by default) and types the code.

The first time, the host also downloads the emulator core from the website (My device) or with `device core download`, and picks the ROM folder. See [emulator.md](emulator.md).

## 2. Linking a browser with the device

```
Device ──register {device_id, device_secret}──► signalhub ──code "113 134 323"──► Device (shows it)
Browser on /device ──claim "113134323"──► signalhub
signalhub ──paired {session_id}──► Device and Browser
Device ⇄ Browser: WebRTC (data channels only)
Browser ──auth {}──► Device (control channel, encrypted)
Device ──auth_ok {device_id, link_id, token}──► Browser (kept in localStorage)
```

A code is used **once per browser**. After that the link is remembered on both sides:

- **Browser:** keeps the `device_id`, `link_id`, `token` and signaling server in `localStorage`.
- **Device:** keeps only the **SHA-256 hash** of each browser's token in `device.json` (`links`), never the token.

After a reload, or when the device restarts, they reconnect by themselves, **without a code**:

```
Browser ──reach {device_id}──► signalhub ──reached──► Device
signalhub ──paired {session_id}──► Browser
Device ⇄ Browser: WebRTC (data channels only, no data yet)
Browser ──auth_challenge {link_id, nonce}──► Device
Device ──auth_proof {link_id, proof}──► Browser   (proof = HMAC-SHA256(SHA-256(token), nonce))
Browser ──auth {link_id, token}──► Device (constant-time hash comparison)
Device ──auth_ok {device_id, link_id}──► Browser   (or auth_failed, and the device hangs up)
```

- **The device proves itself first.** Before showing its token, the browser sends a random nonce, and the device answers with an HMAC that only someone holding the token's hash can compute. If it does not match, the browser hangs up **without sending the token**. So even someone impersonating the device on the signaling server never gets the token. The website computes SHA-256 and HMAC in plain TypeScript (`packages/shared/src/hmac.ts`), so it also works on the local panel over `http`.
- signalhub binds each `device_id` to its `device_secret`: nobody else can register with that `device_id`.
- A browser that arrives with `reach` **gets nothing** (no status, no actions, no files) until the device checks its token. Without a token within 15 seconds, the device hangs up.
- If the device is off, signalhub answers `device not connected` and the website retries every few seconds, showing "device offline".
- **Unlink:** from the website (`unlink`, only that browser) or from the device window (**Unlink all**, which shows a code again). The device answers `unlinked` and the website forgets the link.
- **The device window** shows only the code and the website address while no browser is linked. With at least one, it shows the dashboard; **Link another browser** shows a new code.

## 3. Game rooms

The device is a small **game server**: each room is one game, and several can run at once (`max_rooms`, 4 by default), plus the permanent test pattern room.

1. From a linked browser the host picks a game from the device's library, a room name, voice on or off and the chat on or off. The website sends `create_room` over the `control` channel.
2. The device starts the game in its own emulator process. Only if the game starts does it open the room on signalhub (`room_open`, invite only) and answer `room_created`. If the core cannot run the ROM, it answers `room_error`.
3. signalhub creates the `room_id` and an **invitation** for it.
4. The host shares the invitation, and each guest also gets their own PIN (next section).

Room states:

| State | Meaning |
|---|---|
| `live` | Running |
| `paused` | Paused for everyone; the emulator process stays alive |
| `archived` | Off: the game state is saved and the process ends |
| `trash` | In the trash for 30 days, then deleted with its saves |

Favorites work in any state. Turning an archived room on again offers: **continue** where it stopped, start **fresh**, or load a **saved slot**. Games that the emulator cannot save completely always start fresh (see [emulator.md](emulator.md#save-states)). Rooms that were running when the device stopped come back by themselves when it starts again.

## 4. Invitations and PINs: one invitation, one person

Every room is private. It never appears in a public list.

- **The door:** an invitation link `/g/<invite>` (also as a **QR**) or a **9-digit code** to type in "Join a game". It only says which room to go to. **New link** replaces the link, QR and code, and the old ones stop working. People already inside stay.
- **The key:** a **6-digit PIN per invitation**, made by the device every time the host invites someone. The first browser that uses a PIN gets in, and the PIN is spent. Anyone else trying it gets "someone already joined with this invitation, ask for a new one". An unused PIN lasts 6 hours.
- **Coming back:** when a guest gets in, the device gives them a **return token**. The website keeps it per room (`go-link.room-passes`, the last 20) and sends it instead of the PIN after a reload or a dropped connection.
- **The host's browsers** send the room's **owner key** (`owner_key`), which the device only shows over the linked connection.
- PINs, tokens and owner keys live **in memory only**, and all of them end with the room's session.
- **The device enforces it**, not the website or signalhub: a guest that has not passed the PIN gate gets no WebRTC offer and no seat, and every other message from them is ignored.
- Limits: 5 wrong tries per guest; 20 failures in 10 minutes lock the room for 10 minutes. Comparisons are constant time.
- The PIN never goes in a URL. The guest page keeps it in the navigation state.

The PIN travels through signalhub inside `signal` (TLS to the server). Whoever runs the signaling server could see it, as they could see the whole negotiation. If that matters, use your own server (see [networking.md](networking.md#your-own-signaling-server)).

## 5. Owner view and guest view

A browser **without its own device** (not linked and not remembered) is in **guest mode**: it only sees the "Join a game" form (code or link + PIN) and a **Guest** badge, never a list of games. Room details (name, game, host, artwork) reach a guest over the `control` channel (`room_state.info`) only after the PIN.

The room page depends on **who you are**, not on the URL. The **owner** (a browser linked to the device that runs that room) also sees Invite, Close game and the chat switch. **Everyone else** gets the simple view: video, players, chat (if on), controls, Start 1P-4P, swap controller and voice.

## 6. Playing

- Device → guest: **video** (VP8) and **game audio** (Opus).
- Guest → device: the **`input`** data channel (buttons and sticks, no retransmits).
- Both ways: the **`control`** data channel (chat, room state, queue; reliable).

### Seats and the arcade queue

- Ports P1 to P4 are fixed seats. Someone who joins takes the first free port.
- A room has as many seats as its game has players (2 for Street Fighter II, 4 for Teenage Mutant Ninja Turtles; 4 when the game does not say). With every seat taken, newcomers wait **in the queue**: they watch, chat and see their position.
- When a player leaves, the **first in the queue takes that same port**.
- Spectators only watch and chat (`spectate`, and `queue` to get back in line).
- Only seated players' input reaches the game. The device enforces it.

### Arcade panel extras

- **Start buttons 1P-4P:** any seated player can press any port's Start, like the row of Start buttons on an arcade cabinet (to start a two-player game, or let a friend in).
- **Swap controllers:** a player asks to swap seats. A free seat or one from the same browser swaps at once; someone else's seat asks that person, who has 30 seconds to accept. Crossed requests swap at once.
- **Pause:** a seated player can pause the game for everyone. The chat says who paused.
- **Several players in one browser (host only):** by default the keyboard and every gamepad drive one player, so a pad plugged in by mistake never takes a new seat. The host's **Several controllers** switch in Controls (kept for the browser tab's session, `go-link.multi-pads` in sessionStorage) makes each gamepad a local player, and the keyboard plays as the chosen one. The device enforces it: a guest's browser takes one seat however many local players it sends (`RoomManager.seatable`).

### Voice between players

- Only **seated players (P1 to P4)** can talk. Spectators and the queue use the chat.
- Each player sends their microphone as an Opus track to the device.
- The device forwards each microphone's RTP packets to the **other seated players** (SFU-style: no decoding, no mixing).
- Every connection carries all four voice tracks from the first offer, so taking or leaving a seat never renegotiates.
- The website has separate volumes for game sound and voice, and can silence each player.
- Room option `spectators_hear_voice` (off by default) lets spectators listen without talking.
- The device **drops** audio from any peer without a seat: the rule is enforced on the device, not just in the UI.

### Chat

Up to 300 characters per message, 5 messages every 5 seconds, the last 50 on joining, a "typing…" indicator, and the host can switch the room's chat off (the device then drops chat and typing messages).

## 7. The test pattern room

A permanent diagnostic room, like the echo test of a VoIP service. It streams a PM5544-style **test card** drawn in code at 60 fps: a clock in the top box proves the picture is live, and a drawn gamepad, the local player lamps and the measured frame rate in the bottom box light up as you press keys or a real gamepad. It also plays a **1 kHz reference tone** at -18 dBFS. It checks the website, signalhub, WebRTC, video, sound and controls end to end. It is private like every room (invited from My device). `--test-room=false` turns it off.
