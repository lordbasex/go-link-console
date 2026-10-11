# go-link documentation

| Document | What it covers |
|---|---|
| [Architecture](architecture.md) | Principles, components, repository layout, identifiers and technical decisions |
| [How it works](flows.md) | Linking a browser, game rooms, invitations and PINs, owner and guest views, playing, voice, the test room |
| [Device protocol](protocol.md) | WebRTC channels and tracks, negotiation, the PIN gate, the `input`, `control` and `files` channels, the 2x video scale, the latency test |
| [Room telemetry](telemetry.md) | What the device records about each room (SQLite), the incidents and their verdicts, the network report and `device telemetry` |
| [The device](device.md) | Running the host app, flags, CLI, `device.json`, the native window, the local web panel, Docker |
| [Emulator](emulator.md) | libretro, where ROMs come from, ROM validation, thumbnails, save states, core patches |
| [Video quality lab](quality.md) | `framelab`: reference frames from the core, the device's encoder path, decoding like a browser, picture metrics |
| [The website](web.md) | Development, routes, languages, pages, mobile, design system |
| [The Player apps](mobile.md) | go-link Console Player for Android and iOS: joining by QR code or code + PIN, App Links and Universal Links, the room, controllers, voice, permissions, building and signing |
| [Gamepad skins](skins/README.md) | How skins work and how to design one: the JSON format ([schema](skins/skin.schema.json)), the built-in skins and the visual editor |
| [Networking](networking.md) | STUN/TURN from signaling, your own signaling server, direct connections through one UDP port |
| [Security](security.md) | Signaling, device and website security, HTTP headers |
| [Building](building.md) | Requirements, Makefile, macOS app, releases, tests, CI |
| [Deploying](deploy.md) | Signaling server, website hosting, device distribution |
| [Arcade ROM project](rom/README.md) | go-link's own arcade game for mame2003-plus: a CPS-1 prototype that runs on the stock core and in rooms, recognized by the device as a go-link set, its [lab journal](rom/journal.md) and the [game bible](rom/story.md) with the story and its ten levels |
| [Willy Maker](willy-maker/README.md) | The visual game maker, on its own site ([maker.go-link.org](https://maker.go-link.org), `frontend/willy-maker`): build CPS-1 games, play-test them in the browser, export a project or an AI pack, and validate a ROM in four levels, up to a power-on test on the linked device; creating the ROM in the browser comes later ([architecture](willy-maker/architecture.md), [file format](willy-maker/file-format.md), [validation](willy-maker/validation.md)) |
| [Status and roadmap](status.md) | What is done and what comes next |
| [Legal](legal.md) | Terms of use, privacy policy, licenses and trademarks |

The signaling protocol lives in [signalhub's README](https://github.com/lordbasex/signalhub). Changes are recorded in [CHANGELOG.md](../CHANGELOG.md).
