# Security

## Signaling (signalhub)

Details in [signalhub's README](https://github.com/lordbasex/signalhub). In short:

- Single-use pairing codes that expire, cryptographically random `session_id`, relay only within the same session, `From` always set by the server, `Origin` check, message size limits, TLS (`wss://`).
- `register` requires the `device_secret`, and signalhub binds each `device_id` to it: nobody else can register as that device.
- Rate limits on `claim` and `join`, per-IP and total connection caps, a per-connection message rate, and a limit on room and invitation creation. IPv6 is limited by `/64`.
- TURN credentials are short-lived and unique to each connection; coturn has quotas, no TCP relay, and never relays into private or reserved networks.

## Device

- **With a window it opens no port.** The window uses the services inside the same process, and browsers reach the device only over WebRTC after linking with a code.
- **Headless**, it opens the local panel (:7373): only the website files and a WebSocket for the WebRTC negotiation. Login is a nonce + HMAC proof of the panel token (the token never travels), same-panel `Origin` only, and an IP block after 5 wrong proofs in a minute.
- **Linked browsers:** only the SHA-256 of each token is stored. On a reconnect the device proves itself first (HMAC challenge), and a browser that has not shown a valid token within 15 s gets nothing and is dropped.
- **Rooms:** every room is private. One PIN per invitation and per person, a return token per guest, an owner key per room session, all in memory only. 5 tries per guest, and 20 failures in 10 minutes lock the room for 10 minutes. The owner key and return tokens are checked before the lockout, so an attack does not lock out the host or admitted guests.
- **Room guests never see device data** (hardware, usage, history, IPs); only linked browsers do. The history with guests' IPs is visible only to the owner.
- **Input and voice rules are enforced on the device:** only seated players' input reaches the game, and only seated players' voice is forwarded.
- **Uploads:** strict set names, ZIP signature check, size limit, temporary file + rename, never overwrites.
- **Thumbnails:** size limits on files and pixels before decoding.
- `device.json`, `history.json` and the recordings (`rec/`) are `0600`. `device.lock` prevents two devices at once.
- Recordings are served only to linked browsers that proved their link, only on their `files` channel and only by recording id (`<room id>/<file>.webm`, checked against a strict pattern): never a path. The device serves no recording to room guests.
- **Updates:** the device installs an update only when the release's `SHA256SUMS` carries the update key's Ed25519 signature and the downloaded file has the hash it lists. The private key is on the release computer only, never in the repository or CI, so someone who takes over the GitHub account or the release's files cannot push an update to the devices. On macOS the new app must also carry a valid signature by the running app's developer team. An archive for Windows or Linux may only hold plain files at its top (no folders, no paths that leave it). A file that fails a check is deleted and nothing is installed, and nothing is installed without the host choosing **Update**. **Residual risk:** the release computer itself, and a release key that leaks would need a new key in a new version (devices trust the key their version carries).

## Website

- Never handles ROMs.
- Strict Content Security Policy, with `connect-src wss:` to allow self-hosted signaling servers. No inline scripts or styles.
- The signaling server is kept in `localStorage` and changed only by hand, never from a link.
- PINs are never put in URLs.
- No third-party requests where games, keys or PINs are: fonts are bundled, and the rooms' site (play.go-link.org), the device's panel and the pages that carry the device's keys (`/handoff`, `/maker-bridge`) load no analytics, ads or trackers. Only console.go-link.org's public pages may load Google Analytics 4, and only after the visitor accepts the cookie notice; once it is loaded, moving to a page that is not public is a full page load (`src/analytics.ts`), and the server's CSP header there leaves Google out.
- Text from other users is always rendered as text.
- SHA-256 and HMAC are implemented in TypeScript (`packages/shared/src/hmac.ts`), because `crypto.subtle` is missing on plain `http` pages such as the local panel.

## The two sites and the handoff

The website is two sites ([web.md](web.md#two-sites)): the landing, guide and tools on console.go-link.org, and the rooms and My device on play.go-link.org. The device link and the room passes live only in play's storage; the landing keeps no link and never connects to the signaling server, so a flaw in the landing, the guide or a tool cannot reach a device.

Browsers that used go-link Console before the split still keep those on go-link.org. Moving them (`apps/web/src/handoff.ts`):

- Play opens the landing's `/handoff` only on a click, in a tab with a random name. The handoff page posts its data with play's origin as `targetOrigin`, so if any other page opened it, the browser delivers nothing; it acts on play's `done` only from play's origin and from the tab that opened it.
- Play accepts the data only from the landing's origin and from the tab it opened, takes only `go-link.*` keys (at most 64, each value at most 256 KB), never the landing's own (Willy Maker's games, skins), and never replaces a value it already has.
- Once play has them, the landing deletes the device link and the passes. Nothing goes in the address: `?import=1` only says that there is something to bring.

## Willy Maker's site and the device bridge

Willy Maker is its own site (maker.go-link.org). The link to the owner's go-link Console stays on the rooms' site (play.go-link.org), whose storage another origin cannot read, so Willy Maker reaches the device through a tab of that site it opens, `/maker-bridge`, with `postMessage` (`packages/shared/src/maker-bridge.ts`, [the architecture](willy-maker/architecture.md#the-device-bridge)):

- **Who may talk:** the bridge accepts a message only from Willy Maker's origin (`MAKER_URL`, pinned by `make web-build`, never a development address from a `.env.local`) and only from the tab that opened it; every answer is posted only to that origin, so a third party that opens `/maker-bridge` gets nothing (not even the device's name). Willy Maker's side accepts only the origin of the tab it opened (play.go-link.org, or console.go-link.org to bring the games made there before it moved) and only that tab, whose window name is random.
- **What passes:** a ROM test (`rom_test`) and a private room for the Willy Maker game (`create_room` for `@maker`), their zips (purpose `rom_test` or `maker`, at most 16 MB), and back only `upload_result`, `rom_test_result`, `room_created` and `room_error`. Nothing else of the device: unlink, settings, factory reset, status, other rooms.
- **Messages are rebuilt, never passed on:** the bridge builds a new control message from the allowed fields, each type-checked (`bridgeControl`). Go's `encoding/json` matches keys in any case and keeps the last one, so a message with `"type":"rom_test"` and `"TYPE":"factory_reset"` would have reached the device as a factory reset (found in the review of the bridge, 2026-10-05). The device also drops, before reading it, any control message with two keys equal but for case, at any depth (`pkg/jsonkeys`), whoever sends it.
- On the device, a `@maker` room is always private, gets a new id from the device and only replaces the Willy Maker room before it; a test or game zip never enters the ROM folder.
- Both sites send `frame-ancestors 'none'` and `X-Frame-Options: DENY`, so neither the maker nor the bridge can be framed.

## HTTP headers for the website

A `<meta>` CSP cannot set everything. Whatever static host or CDN serves the website should add these headers to every response:

| Header | Value | Why |
|---|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self'; font-src 'self'; img-src 'self' data: blob:; connect-src 'self' wss:; media-src 'self' blob:; base-uri 'none'; form-action 'self'; object-src 'none'; frame-ancestors 'none'` | Own scripts and styles only (plus WebAssembly, for the in-browser MP4 conversion), WebSockets only over `wss:`, and no one can frame the site |
| `X-Frame-Options` | `DENY` | The same for older browsers (clickjacking) |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Always HTTPS |
| `X-Content-Type-Options` | `nosniff` | No content type guessing |
| `Referrer-Policy` | `no-referrer` | Invitation links do not leak to other sites |
| `Permissions-Policy` | `microphone=(self), camera=(), geolocation=(), payment=()` | The microphone is for voice between players and for dictation in Willy Maker (Chrome's on-device recognition only) |

If the CSP in `frontend/apps/web/index.html` changes, change it on the host too. Willy Maker's site gets the same headers (its own CSP is in `frontend/willy-maker/vite.config.ts`, with `connect-src 'self'`: it connects to nothing else).

## Repository hygiene

- Secrets never go in the repository: `.env` files are gitignored (only `.env.example` templates are committed), and hosting settings live in the gitignored `deploy/local/`.
- CI runs gitleaks over the whole history on every push. Fake tokens in tests carry a `gitleaks:allow` comment.
- CI runs `govulncheck` for Go and `npm audit` for the website.

## Reporting a vulnerability

Please report security issues privately by email to lord.basex@gmail.com instead of opening a public issue.
