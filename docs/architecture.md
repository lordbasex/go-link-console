# Architecture

go-link Console is **host streaming**, not netplay. One person, the **host**, runs the go-link Console app (the **device**) on their own computer with **their own ROMs**. Everyone else plays from a web browser: they get the game's video and sound over WebRTC and send their controls back. Nobody but the host needs the ROM.

## Principles

| Principle | What it means |
|---|---|
| **No ROMs on any server** | ROMs live only on the host's disk. No central component receives, stores or relays them. |
| **Peer to peer** | Video, sound, voice and controls travel directly between the device and each browser (WebRTC). Servers only introduce the parties. |
| **Generic signaling** | The signaling server knows nothing about MAME. It is [signalhub](https://github.com/lordbasex/signalhub), a separate project shared with other apps. |
| **For non-technical hosts** | The device is a native app with a window and a tray icon. No terminal needed. |
| **Everything also from the CLI** | Every device feature also works from the command line, for a Raspberry Pi or a server without a screen. |
| **Business logic over WebRTC** | Rooms, ROMs, history and settings travel over WebRTC data channels. The device exposes no REST API. |

## Big picture

```
┌────────────────────────────┐           ┌──────────────────────────────┐
│  frontend/apps/web         │           │  signalhub (own repository)  │
│  React, static website     │◄── WSS ──►│  Go + WebSocket              │
│  - /device (9-digit code)  │           │  - pairing by one-time code  │
│  - /g/:invite (join)       │           │  - rooms and invitations     │
│  - video, gamepad, chat,   │           │  - relays opaque SDP / ICE   │
│    voice between players   │           │  - STUN/TURN credentials     │
└─────────────▲──────────────┘           └───────────────▲──────────────┘
              │                                          │ WSS (outgoing)
              │  WebRTC, peer to peer                    │
              │  video/audio ▼    controls/chat ▲        │
┌─────────────┴──────────────────────────────────────────┴─────────────┐
│  backend-device/  (native Go + cgo app: Windows, Linux, macOS)       │
│  - MAME through libretro (mame2003-plus), one process per game room  │
│  - VP8 + Opus encoding, WebRTC with Pion                             │
│  - rooms, seats P1-P4, arcade queue, chat, voice (SFU-style)         │
│  - ROM library, ROM validation, thumbnails, save states, history     │
│  - native window (Fyne) + tray icon, or headless with a LAN panel    │
└──────────────────────────────────────────────────────────────────────┘

          coturn (STUN/TURN) next to signalhub, to get through NAT
```

## Components

| Directory | Language | Runs on | Responsibility | Docs |
|---|---|---|---|---|
| signalhub (own repository) | Go | Any Linux server (Docker) | Pair device and browser, rooms and invitations, relay WebRTC messages, TURN credentials. Nothing else. | [signalhub](https://github.com/lordbasex/signalhub) |
| `backend-device/` | Go + C (cgo) | The host's computer | Emulation, streaming, rooms, queue, chat, voice, ROMs, native GUI and CLI | [device.md](device.md), [emulator.md](emulator.md) |
| `frontend/` | React + TypeScript | Any static web host | The website for players, guests and hosts | [web.md](web.md) |
| `e2e/` | TypeScript (Playwright) | Developer machine, CI | End-to-end tests in a real browser | [building.md](building.md#end-to-end-tests) |
| `cores/mame2003-plus/` | Patches + shell | Developer machine | Optional save state patches for the emulator core | [README](../cores/mame2003-plus/README.md) |

Star topology everywhere: guests only talk to the device. signalhub enforces it for `signal` messages, and the device forwards voice SFU-style without mixing.

## Repository layout

```
go-link/
├── README.md                 # summary and quick start
├── CHANGELOG.md
├── docs/                     # this documentation
├── backend-device/           # the device (Go + cgo)
│   ├── cmd/device/           # entry point, CLI subcommands, the emulator worker
│   ├── internal/             # models, repositories, services, gui, panel
│   ├── pkg/                  # libretro, encoder, signalclient, romcheck, thumbnails...
│   ├── build/                # Dockerfiles, macOS app bundle and dmg
│   └── test/                 # integration tests
├── frontend/                 # npm workspaces
│   ├── apps/web/             # the website
│   └── packages/shared/      # protocol types, signaling client, WebRTC player, tokens
├── e2e/                      # Playwright end-to-end tests
├── cores/mame2003-plus/      # optional core patches, applied at build time
├── scripts/release.sh        # release packaging
├── Casks/                    # Homebrew cask
└── .github/workflows/ci.yml
```

Go code follows the same layout in every module: `cmd/` (entry points), `internal/` (`models`, `services`, `repositories`…), `pkg/` (reusable packages) and `test/` (integration tests with `httptest.NewServer` and real WebSocket clients).

## Identifiers

Keeping these apart is the base of the whole design:

| Identifier | Example | Created by | Lifetime | Secret? | Used for |
|---|---|---|---|---|---|
| `device_id` | UUID v4 `7f3c…` | The device, on first run | Permanent (on disk) | No | Identifies the host's computer |
| `device_secret` | 32 random bytes | The device, on first run | Permanent (on disk, 0600) | **Yes** | Proves the `device_id` to signalhub |
| Pairing code | `113 134 323` | signalhub | 10 min, **single use** | Semi | Human bridge to link a browser with the device (RFC 8628 style, like TV apps) |
| `room_id` | UUID v4 | signalhub (when a room opens) | While the room lives | Semi | Internal room address. Random: cannot be guessed or enumerated |
| Invitation | 22-char token + 9-digit code | signalhub | While the room lives, until replaced | Semi | The door to a private room: `/g/<invite>`, a QR or a code |
| Invitation PIN | 6 digits | The device | Single use, 6 h if unused | **Yes** | The key: lets exactly one person in |
| `session_id` | 32 random hex | signalhub | While connected | **Yes** | Groups every peer of a session |
| `peer_id` | 32 random hex | signalhub | One WebSocket connection | No | Address to relay messages to |

## Technical decisions

| # | Decision | Rejected alternative | Why |
|---|---|---|---|
| 1 | **Host streaming** | Lockstep netplay | With netplay every player needs the ROM. With streaming only the host does. |
| 2 | **libretro** as the bridge to MAME | Linking MAME directly | MAME is C++. libretro is a plain C API, a good fit for cgo. |
| 3 | **mame2003-plus** core | Current MAME | Light and fast for classic arcade games, and it runs on a Raspberry Pi. |
| 4 | **One emulator process per game room** | Every game in the device process | A libretro core allows one instance per process, and a crashing game cannot take the others down. |
| 5 | **Native GUI with Fyne**; a **local web panel only when headless** | Always a web panel, Wails | The window calls the device services directly, so there is no HTTP port to protect. Headless devices serve the website on the LAN, and business logic still goes over WebRTC. |
| 6 | Signaling **in memory, no database** | Postgres or Redis | Everything is ephemeral and one process is enough for several projects. After a restart, devices reconnect by themselves. |
| 7 | **Pion** (pure Go) for WebRTC | libwebrtc (C++) | Native Go, well maintained, no Chromium build. |
| 8 | **Native binary** for the device | Docker only | Docker on Windows and macOS runs in a VM, which adds latency and complicates networking. A Docker image exists for Linux servers. |
| 9 | `room_id` is a **server-made UUID v4** | Short room codes | Unique and not enumerable. The server creates it, so a device cannot claim someone else's room. |
| 10 | **Every room is private**: invitation + one PIN per person | Public lobby | Only people the host invites can join, and a leaked link alone is not enough. |
| 11 | **Voice forwarded by the device** (star) | P2P mesh between players | The device controls who may talk, consistent with the rest of the design. |
| 12 | Clients take STUN/TURN from signalhub's `hello` | STUN/TURN in the clients' code | Changing the signaling server changes everything, and TURN credentials are short-lived. |
