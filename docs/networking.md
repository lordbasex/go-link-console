# Networking

## Nothing hardcoded in the clients

Neither the device nor the website has STUN or TURN servers in its code. All they know is **the signaling server URL**:

```
device or website ──connects (wss://)──► signalhub
signalhub ──hello {peer_id, ice_servers}──► device or website
```

- `ice_servers` carries the STUN and TURN URLs, and for TURN a short-lived username and password, unique to that connection.
- Clients keep them **in memory only** and use them for every `RTCPeerConnection`. Each reconnect brings fresh ones.
- **Device:** uses the public server by default; `--server-signaling` or `signal_url` in `device.json` changes it (see [device.md](device.md#which-signaling-server-it-uses)).
- **Website:** uses the server set at build time (`VITE_SIGNAL_URL`); users can switch from the website itself.

## Your own signaling server

signalhub is open source ([lordbasex/signalhub](https://github.com/lordbasex/signalhub)) and runs on any Linux server with Docker. Its README explains the ports to open and the variables to set.

On the website, the **Signaling server** button lets a user switch to another server:

1. The user types the URL (`wss://signal.example.com/ws`).
2. The website checks it and **tests the connection** before saving: it must connect and receive `hello`. If not, nothing is saved.
3. It is saved in the browser's `localStorage`. From then on the website uses that server for everything, including the STUN/TURN it sends.
4. While a non-official server is active, the website always shows it, with a **Back to the official server** action.

Rules:

- Only `wss://` is accepted (an HTTPS page cannot open plain `ws://`; `ws://` to the local machine is allowed in development).
- The server is only changed **by hand** from that button, never from a link parameter: otherwise a link could send other users to a malicious server.
- A self-hosted server must allow the rooms' site origin in `ALLOWED_ORIGINS` (`https://play.go-link.org`; the landing never connects) and the app `go-link-console` in `ALLOWED_APPS`, and allow several rooms per session (`MAX_ROOMS_PER_SESSION` > 1).
- The device and the players must use **the same server**: the host sets it on the device and each player in their browser.
- The CSP allows `connect-src` to any `wss:` precisely to allow this.
- When an invitation or pairing code is not found, the website explains that the host may be using their own signaling server and offers a shortcut to that setting.

## Direct connections: one UDP port

WebRTC looks for a direct path between the browser and the device. If there is none, it uses the signaling server's **TURN relay**, which always works but adds the round trip to the server and uses its bandwidth. The room shows it next to the latency, `(direct)` or `(relay)`, and the device logs it per connection (`viewer path`).

This happens with cascaded routers (the device behind a second router, players on the first router's network) or internet players behind a double NAT. Browsers hide their local address with `.local` (mDNS) names, which do not cross routers.

The fix is a **fixed UDP port** forwarded on the router:

1. `udp_port` in `device.json` (or `--udp-port 50000`): every WebRTC connection, for every room, goes through that single UDP port.
2. On the router in front of the device, forward that UDP port to the device's address.
3. `announce_ips` in `device.json` (or `--announce 192.0.2.10`): the addresses where browsers reach that port (the inner router's WAN side for a home network, the public address for the internet). The device sends them as extra ICE candidates with the same port.

```json
{ "udp_port": 50000, "announce_ips": ["192.168.1.50"] }
```
