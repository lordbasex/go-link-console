# Deploying

go-link has three pieces to put in front of people:

| Piece | Where it runs | How |
|---|---|---|
| Signaling server (signalhub + coturn) | Any Linux server with Docker | From [its own repository](https://github.com/lordbasex/signalhub) |
| Website: the landing and the rooms | Any static web host or CDN, each on its own host name | `make web-build`, then upload `frontend/apps/web/dist-site` and `dist-play` |
| Willy Maker's site | Any static web host or CDN, on its own host name | `make maker-build`, then upload `frontend/willy-maker/dist` |
| Device | The hosts' computers | GitHub releases (`make release`) |

## Signaling server

signalhub is a separate, generic project. Its README has the step-by-step deployment, the ports to open (TCP 80/443, TCP and UDP 3478, the UDP relay range) and every variable. Its server builds the image from source with `docker compose`; no image is published to a registry.

For go-link, set in its `deploy/.env`:

| Variable | Value for go-link |
|---|---|
| `ALLOWED_ORIGINS` | The rooms' site origin, for example `https://play.go-link.org` (plus `http://localhost:5180` for development). The landing (`https://console.go-link.org`) never talks to signaling |
| `ALLOWED_APPS` | Must include `go-link` |
| `MAX_ROOMS_PER_SESSION` | More than 1: each game room of a device is a room of the same session |

Then point the website (`SIGNAL_URL` at build time) and the devices (`--server-signaling` or `signal_url`) at `wss://<your domain>/ws`.

## Website

The website is a static single-page app built twice ([two sites](web.md#two-sites)): the landing, guide and tools (console.go-link.org) and the rooms and My device (play.go-link.org):

```bash
make web-build                                      # frontend/apps/web/dist-site and dist-play
SIGNAL_URL=wss://signal.example.com/ws make web-build   # for your own signaling server
SITE_URL=https://example.org PLAY_URL=https://play.example.org make web-build   # your own host names
```

Each site trusts the other's address for the handoff, and the landing sends the rooms' paths there, so both are built together with `SITE_URL` and `PLAY_URL` (and `INVITE_URL`, the address invitations are shared with, default `https://go-link.org`). Upload the rooms' site first. The rooms' site sends `noindex`.

Since 2026-10-11, go-link.org (and www) is an index of go-link's WebRTC projects, deployed apart from this repository. It serves `.well-known/` (the apps' invitation links stay on `go-link.org/g/<invite>`), sends `/g/*` to play and every other path to the landing at console.go-link.org. Your own host only needs the index when your Player apps open your invitation address.

Whatever serves them must:

1. **Serve `index.html` for every route** (`/rooms`, `/device`, `/g/<invite>`…): unknown paths return `index.html` with status 200.
2. **Cache** `assets/` (hashed file names) for a year (`public, max-age=31536000, immutable`) and **never cache** `index.html` (`no-cache`).
3. **Add the security headers** listed in [security.md](security.md#http-headers-for-the-website).
4. Serve over **HTTPS**.
5. Never publish `.map`, `.env` or other local files (`make web-build` already removes source maps).

`make web-deploy` builds the site and runs the upload defined in **`deploy/local/hosting.mk`**. That folder is gitignored, because upload commands, bucket or server names and account details belong to each deployment, not to the repository. Start from the example:

```bash
mkdir -p deploy/local
cp deploy/hosting.example.mk deploy/local/hosting.mk   # an rsync-over-SSH example
make web-deploy WEB_SERVER=user@host WEB_ROOT=/var/www/go-link
```

The file can define `web-deploy`, a `hosting-help` target (shown by `make help`) and `HOSTING_TARGETS = 1`, and it may include private settings from a gitignored `.env.deploy`. Without it, `make web-deploy` only builds the site and says where it is.

### Willy Maker's site

Willy Maker is its own static site (`frontend/willy-maker`), served on its own host name (go-link's is `maker.go-link.org`):

```bash
make maker-build                                    # frontend/willy-maker/dist
SITE_URL=https://go-link.example.org make maker-build   # with your own website
make maker-deploy                                   # build and upload (deploy/local/hosting.mk)
```

- It reaches the owner's go-link through the rooms' site `/maker-bridge` tab, so each must know the other: Willy Maker is built with the rooms' address (`PLAY_URL`, default `https://play.go-link.org`) and the landing's (`SITE_URL`, default `https://console.go-link.org`, for the games made there before it moved), and the website with Willy Maker's (`VITE_MAKER_URL`, default `https://maker.go-link.org`).
- Serve it like the website: `index.html` for unknown paths (a game's address is `/<id>`), `index.html` with `no-cache` and `assets/` cached for a year, HTTPS only, and the same security headers (`frame-ancestors 'none'` and the rest, [security.md](security.md)).
- `hosting.mk` can define `maker-deploy` the same way as `web-deploy`; without it, `make maker-deploy` only builds the site.

## Device

Hosts download the device from the GitHub releases: one universal `.dmg` for macOS (Intel and Apple silicon, macOS 12 or later), a `.zip` for Windows, `.tar.gz` for Linux (with and without window) and optionally a Docker image. See [building.md](building.md#releases) for `make release`, and [device.md](device.md#docker) for running the Docker image on a server or a Raspberry Pi.

The device's defaults point to the project's public services (`wss://signal.go-link.org/ws` and `https://play.go-link.org`, the rooms' site). A self-hosted setup changes them with `signal_url` and `web_url` in `device.json`.
